package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Server runs the outer SSH server.
type Server struct {
	cfg       *Config
	log       *slog.Logger
	store     *sessionStore
	repo      *repo
	dialer    *nsProxyDialer
	resolver  vmAddressResolver
	sshConfig *ssh.ServerConfig
}

func NewServer(cfg *Config, log *slog.Logger, store *sessionStore, r *repo, resolver vmAddressResolver) (*Server, error) {
	hostKey, err := LoadOrCreateHostKey(cfg.HostKeyPath)
	if err != nil {
		return nil, err
	}
	dialer, err := newNsProxyDialer(cfg.NsProxySock, 10*time.Second)
	if err != nil {
		return nil, err
	}
	sc := &ssh.ServerConfig{
		KeyboardInteractiveCallback: kbdInteractiveAuthenticator(cfg, store),
	}
	sc.AddHostKey(hostKey)
	return &Server{
		cfg:       cfg,
		log:       log,
		store:     store,
		repo:      r,
		dialer:    dialer,
		resolver:  resolver,
		sshConfig: sc,
	}, nil
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	var wg sync.WaitGroup
	defer wg.Wait()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			s.log.Warn("accept", "err", err)
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handle(ctx, c)
		}()
	}
}

func (s *Server) handle(ctx context.Context, raw net.Conn) {
	defer func() { _ = raw.Close() }()
	_ = raw.SetDeadline(time.Now().Add(s.cfg.NonceTTL + 30*time.Second))
	conn, chans, reqs, err := ssh.NewServerConn(raw, s.sshConfig)
	if err != nil {
		s.log.Info("handshake failed", "remote", raw.RemoteAddr(), "err", err)
		return
	}
	_ = raw.SetDeadline(time.Time{})
	defer func() { _ = conn.Close() }()
	go ssh.DiscardRequests(reqs)

	email := conn.Permissions.Extensions[permEmailKey]

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		ch, sessionReqs, err := newCh.Accept()
		if err != nil {
			s.log.Warn("accept session", "err", err)
			continue
		}
		go s.handleSession(ctx, conn, ch, sessionReqs, email)
	}
}

func (s *Server) handleSession(ctx context.Context, sshConn *ssh.ServerConn, ch ssh.Channel, reqs <-chan *ssh.Request, email string) {
	defer func() { _ = ch.Close() }()

	var pty pendingPty
	var execCmd string
	var agentForwarded bool

	// Channel-request pump: stops when we either get shell/exec or the channel closes.
	requestQueue := make(chan *ssh.Request, 1)
	go func() {
		for req := range reqs {
			switch req.Type {
			case "pty-req":
				if len(req.Payload) >= 4 {
					termLen := int(binary.BigEndian.Uint32(req.Payload[0:4]))
					if 4+termLen+8 <= len(req.Payload) {
						pty.term = string(req.Payload[4 : 4+termLen])
						pty.cols = int(binary.BigEndian.Uint32(req.Payload[4+termLen : 4+termLen+4]))
						pty.rows = int(binary.BigEndian.Uint32(req.Payload[4+termLen+4 : 4+termLen+8]))
						pty.set = true
					}
				}
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
			case "auth-agent-req@openssh.com":
				agentForwarded = true
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
			case "exec":
				if len(req.Payload) >= 4 {
					n := int(binary.BigEndian.Uint32(req.Payload[0:4]))
					if 4+n <= len(req.Payload) {
						execCmd = string(req.Payload[4 : 4+n])
					}
				}
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
				requestQueue <- req
				return
			case "shell":
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
				requestQueue <- req
				return
			default:
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
			}
		}
	}()

	select {
	case <-requestQueue:
	case <-time.After(15 * time.Second):
		_, _ = fmt.Fprintln(ch, "no shell/exec request — aborting")
		return
	}

	if !agentForwarded {
		_, _ = fmt.Fprintln(ch, "ssh-agent forwarding required. Use `ssh -A`.")
		return
	}

	// Resolve the user's VM list.
	vms, err := s.repo.ListInstancesByEmail(ctx, email)
	if err != nil {
		_, _ = fmt.Fprintf(ch, "lookup failed: %v\r\n", err)
		return
	}
	if len(vms) == 0 {
		fmt.Fprintf(ch, "No instances. Create one at %s\r\n", s.cfg.AuthURLBase)
		return
	}

	// Pick a VM: explicit exec command > single auto-pick > menu.
	var target VM
	switch {
	case execCmd != "":
		v, ok := FindByName(vms, strings.TrimSpace(execCmd))
		if !ok {
			fmt.Fprintf(ch, "VM %q not found among your instances.\r\n", execCmd)
			return
		}
		target = v
	case len(vms) == 1:
		target = vms[0]
	default:
		v, ok := promptForVM(ch, vms)
		if !ok {
			return
		}
		target = v
	}

	// Resolve the VM's fixed IPv4 via OpenStack.
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ip, err := s.resolver.ResolveFixedIPv4(rctx, target.OpenstackID)
	if err != nil {
		fmt.Fprintf(ch, "VM unreachable: %v\r\n", err)
		return
	}

	// Dial via ns-proxy.
	dctx, dcancel := context.WithTimeout(ctx, 15*time.Second)
	defer dcancel()
	tcp, err := s.dialer.Dial(dctx, ip, 22)
	if err != nil {
		fmt.Fprintf(ch, "VM unreachable (ns-proxy): %v\r\n", err)
		return
	}
	defer func() { _ = tcp.Close() }()

	// Borrow the user's agent.
	ag, agentCloser, err := agentClientFromOuter(sshConn)
	if err != nil {
		fmt.Fprintf(ch, "agent forwarding setup failed: %v\r\n", err)
		return
	}
	defer func() { _ = agentCloser.Close() }()

	// Inner SSH handshake. The login user defaults to "root"; OpenStack cloud-
	// init images vary (ubuntu/centos/...) — Phase 1 PoC uses "root" and the
	// operator must ensure the keypair injects to root@.
	innerCtx, innerCancel := context.WithTimeout(ctx, 15*time.Second)
	defer innerCancel()
	inner, err := dialInnerSSH(innerCtx, tcp, "root", ag)
	if err != nil {
		fmt.Fprintf(ch, "VM auth failed: %v\r\n", err)
		return
	}
	defer func() { _ = inner.Close() }()

	if err := pipeSession(s.log, ch, reqs, inner, pty); err != nil {
		s.log.Info("pipe ended", "err", err)
		return
	}
}

// promptForVM renders the menu and reads exactly one line. Re-prompts up to 3
// times on bad input.
func promptForVM(rw io.ReadWriter, vms []VM) (VM, bool) {
	for attempt := 0; attempt < 3; attempt++ {
		RenderMenu(rw, vms)
		buf := make([]byte, 64)
		line, err := readLine(rw, buf)
		if err != nil {
			return VM{}, false
		}
		vm, perr := ParseSelection(line, vms)
		if perr == nil {
			return vm, true
		}
		fmt.Fprintln(rw, "invalid selection, try again")
	}
	fmt.Fprintln(rw, "too many invalid attempts; closing")
	return VM{}, false
}

// readLine reads up to a CR or LF from r. Treats CRLF and LF identically.
func readLine(r io.Reader, scratch []byte) (string, error) {
	var out strings.Builder
	for {
		n, err := r.Read(scratch[:1])
		if n > 0 {
			b := scratch[0]
			if b == '\n' || b == '\r' {
				return out.String(), nil
			}
			out.WriteByte(b)
		}
		if err != nil {
			return out.String(), err
		}
	}
}
