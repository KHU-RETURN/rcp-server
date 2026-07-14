package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

const copyDrainTimeout = time.Second

// socks5HandshakeTimeout bounds the SOCKS5 CONNECT handshake when the
// caller's context carries no deadline of its own.
const socks5HandshakeTimeout = 15 * time.Second

// vmAddressResolver returns the dial-target IPv4 for an openstack_id. The PoC
// implementation lives in main.go and uses gophercloud; tests can stub this.
type vmAddressResolver interface {
	ResolveVM(ctx context.Context, openstackID string) (VMRuntime, error)
}

// nsProxyDialer wraps the SOCKS5 client over a Unix-socket transport.
type nsProxyDialer struct {
	sockPath string
	timeout  time.Duration
	socks    proxy.ContextDialer
}

func newNsProxyDialer(sockPath string, timeout time.Duration) (*nsProxyDialer, error) {
	unix := &unixDialer{path: sockPath, timeout: timeout}
	socks, err := proxy.SOCKS5("unix", sockPath, nil, unix)
	if err != nil {
		return nil, fmt.Errorf("socks5 setup: %w", err)
	}
	dctx, ok := socks.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("socks5 dialer missing context support")
	}
	return &nsProxyDialer{sockPath: sockPath, timeout: timeout, socks: dctx}, nil
}

// Dial opens a TCP-equivalent connection to host:port via the ns-proxy SOCKS5
// server reachable on the local Unix socket.
func (d *nsProxyDialer) Dial(ctx context.Context, host string, port int) (net.Conn, error) {
	conn, err := d.socks.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if conn != nil {
		// Handshake succeeded; clear the deadline set in unixDialer.DialContext
		// so it doesn't kill the long-lived proxied stream.
		_ = conn.SetDeadline(time.Time{})
	}
	return conn, err
}

// unixDialer satisfies proxy.Dialer/proxy.ContextDialer by always dialing a
// preset Unix socket regardless of the host:port argument from the SOCKS5 lib.
type unixDialer struct {
	path    string
	timeout time.Duration
}

func (u *unixDialer) Dial(network, address string) (net.Conn, error) {
	return u.DialContext(context.Background(), network, address)
}

func (u *unixDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	dctx := ctx
	if u.timeout > 0 {
		var cancel context.CancelFunc
		dctx, cancel = context.WithTimeout(ctx, u.timeout)
		defer cancel()
	}
	var d net.Dialer
	conn, err := d.DialContext(dctx, "unix", u.path)
	if err != nil {
		return nil, err
	}
	// Hold a deadline over the conn until the SOCKS5 handshake that follows
	// (performed by the caller right after this returns) completes;
	// nsProxyDialer.Dial clears it once the handshake succeeds.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(socks5HandshakeTimeout)
	}
	_ = conn.SetDeadline(deadline)
	return conn, nil
}

// dialInnerSSH performs the inner SSH handshake to the VM using an ephemeral
// gateway-managed key and the gateway-managed host key store.
func dialInnerSSH(ctx context.Context, raw net.Conn, addr, user string, signer ssh.Signer, hostKeyCallback ssh.HostKeyCallback) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: innerHostKeyCallbackForAddress(addr, hostKeyCallback),
		Timeout:         15 * time.Second,
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(dl)
		defer func() { _ = raw.SetDeadline(time.Time{}) }()
	}
	c, chans, reqs, err := ssh.NewClientConn(raw, addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("inner ssh handshake: %w", err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func innerHostKeyCallbackForAddress(address string, cb ssh.HostKeyCallback) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		portNumber, err := strconv.Atoi(port)
		if err != nil {
			return err
		}
		return cb(address, &net.TCPAddr{IP: net.ParseIP(host), Port: portNumber}, key)
	}
}

// pipeSession shuttles bytes + window changes between the outer session
// channel and the inner session. Closes innerSess and inner once the outer
// session ends.
func pipeSession(log *slog.Logger, outerChan ssh.Channel, outerReqs <-chan *ssh.Request, inner *ssh.Client, ptyReq pendingPty, closeOuterConn func()) error {
	innerSess, err := inner.NewSession()
	if err != nil {
		return fmt.Errorf("inner session: %w", err)
	}
	defer func() { _ = innerSess.Close() }()

	if ptyReq.set {
		if err := innerSess.RequestPty(ptyReq.term, ptyReq.rows, ptyReq.cols, ssh.TerminalModes{}); err != nil {
			return fmt.Errorf("request pty: %w", err)
		}
	}
	innerStdin, err := innerSess.StdinPipe()
	if err != nil {
		return err
	}
	innerStdout, err := innerSess.StdoutPipe()
	if err != nil {
		return err
	}
	innerStderr, err := innerSess.StderrPipe()
	if err != nil {
		return err
	}

	if err := innerSess.Shell(); err != nil {
		return fmt.Errorf("inner shell: %w", err)
	}

	var wg sync.WaitGroup
	outerInputDone := make(chan struct{})
	wg.Add(3)
	go func() {
		defer wg.Done()
		defer close(outerInputDone)
		_, _ = io.Copy(innerStdin, outerChan)
		_ = innerStdin.Close()
	}()
	go func() { defer wg.Done(); _, _ = io.Copy(outerChan, innerStdout) }()
	go func() { defer wg.Done(); _, _ = io.Copy(outerChan.Stderr(), innerStderr) }()

	// Forward window-change requests from outer to inner.
	go func() {
		for req := range outerReqs {
			switch req.Type {
			case "window-change":
				if len(req.Payload) >= 16 {
					cols := int(binary.BigEndian.Uint32(req.Payload[0:4]))
					rows := int(binary.BigEndian.Uint32(req.Payload[4:8]))
					_ = innerSess.WindowChange(rows, cols)
				}
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
			default:
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
			}
		}
	}()

	innerWait := make(chan error, 1)
	go func() { innerWait <- innerSess.Wait() }()

	exitErr := waitForInnerOrOuter(innerWait, outerInputDone, func() {
		_ = innerSess.Close()
		_ = inner.Close()
	}, 5*time.Second)
	_ = innerStdin.Close()
	_ = outerChan.Close()
	if closeOuterConn != nil {
		closeOuterConn()
	}
	if !waitForCopyDone(&wg, copyDrainTimeout) {
		log.Warn("session copy goroutines did not drain before timeout")
	}
	if exitErr != nil {
		log.Debug("inner session exited", "err", exitErr)
	}
	return nil
}

func waitForInnerOrOuter(innerWait <-chan error, outerDone <-chan struct{}, closeInner func(), timeout time.Duration) error {
	select {
	case err := <-innerWait:
		return err
	case <-outerDone:
		if closeInner != nil {
			closeInner()
		}
		select {
		case err := <-innerWait:
			return err
		case <-time.After(timeout):
			return errors.New("inner session did not exit after outer disconnect")
		}
	}
}

func waitForCopyDone(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// pendingPty captures the outer client's pty-req so the gateway can replay it
// against the inner session. server.go fills this in.
type pendingPty struct {
	set        bool
	term       string
	rows, cols int
}
