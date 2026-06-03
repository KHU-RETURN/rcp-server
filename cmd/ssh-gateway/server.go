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

	"github.com/KHU-RETURN/rcp-server/internal/domain/access"
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
	keyClient ephemeralKeyRegistrar
	sshConfig *ssh.ServerConfig
	hostKeyCB ssh.HostKeyCallback
}

type ephemeralKeyRegistrar interface {
	Register(ctx context.Context, req access.EphemeralAuthorizedKeyRequest) error
	Delete(ctx context.Context, req access.EphemeralAuthorizedKeyRequest) error
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
	hostKeyCB := reloadingInnerHostKeyCallback(cfg.KnownHostsPath)
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
		keyClient: newEphemeralKeyClient(cfg.APIURLBase, []byte(cfg.NotifySecret)),
		sshConfig: sc,
		hostKeyCB: hostKeyCB,
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

	// Resolve the user's VM list.
	vms, err := s.repo.ListInstancesByEmail(ctx, email)
	if err != nil {
		_, _ = fmt.Fprintf(ch, "lookup failed: %v\r\n", err)
		return
	}
	if len(vms) == 0 {
		_, _ = fmt.Fprintf(ch, "No instances. Create one at %s\r\n", s.cfg.AuthURLBase)
		return
	}

	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	vms = applyRuntimeInfo(vms, func(vm VM) (VMRuntime, error) {
		return s.resolver.ResolveVM(rctx, vm.OpenstackID)
	})

	// Pick a VM: explicit exec command > single auto-pick > menu.
	var target VM
	switch {
	case execCmd != "":
		v, ok := FindByName(vms, strings.TrimSpace(execCmd))
		if !ok {
			_, _ = fmt.Fprintf(ch, "VM %q not found among your instances.\r\n", execCmd)
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

	ip := strings.TrimSpace(target.FixedIPv4)
	if ip == "" {
		_, _ = fmt.Fprintf(ch, "VM unreachable: no fixed IPv4 address for %s\r\n", target.Name)
		return
	}

	inner, keyReq, err := s.dialVMWithEphemeralKey(ctx, target, ip)
	if err != nil {
		_, _ = fmt.Fprintf(ch, "VM auth failed: %v\r\n", err)
		return
	}
	defer func() { _ = inner.Close() }()
	defer s.deleteEphemeralKey(keyReq)

	if err := pipeSession(s.log, ch, reqs, inner, pty, func() { _ = sshConn.Close() }); err != nil {
		s.log.Info("pipe ended", "err", err)
		return
	}
}

func (s *Server) dialVMWithEphemeralKey(ctx context.Context, target VM, ip string) (*ssh.Client, access.EphemeralAuthorizedKeyRequest, error) {
	if len(s.cfg.VMUsers) == 0 {
		return nil, access.EphemeralAuthorizedKeyRequest{}, errors.New("no VM SSH users configured")
	}
	var lastErr error
	for _, user := range s.cfg.VMUsers {
		user = strings.TrimSpace(user)
		if user == "" {
			continue
		}
		inner, keyReq, err := s.tryDialVMUser(ctx, target, ip, user)
		if err == nil {
			return inner, keyReq, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no VM SSH users configured")
	}
	return nil, access.EphemeralAuthorizedKeyRequest{}, lastErr
}

func (s *Server) tryDialVMUser(ctx context.Context, target VM, ip, user string) (*ssh.Client, access.EphemeralAuthorizedKeyRequest, error) {
	signer, authorizedKey, err := generateEphemeralSSHKey()
	if err != nil {
		return nil, access.EphemeralAuthorizedKeyRequest{}, fmt.Errorf("ephemeral key setup: %w", err)
	}
	keyReq := access.EphemeralAuthorizedKeyRequest{
		InstanceID:    target.OpenstackID,
		Username:      user,
		AuthorizedKey: authorizedKey,
	}
	kctx, kcancel := context.WithTimeout(ctx, 5*time.Second)
	if err := s.keyClient.Register(kctx, keyReq); err != nil {
		kcancel()
		return nil, access.EphemeralAuthorizedKeyRequest{}, fmt.Errorf("register ephemeral key for %s: %w", user, err)
	}
	kcancel()

	dctx, dcancel := context.WithTimeout(ctx, 15*time.Second)
	defer dcancel()
	tcp, err := s.dialer.Dial(dctx, ip, 22)
	if err != nil {
		s.deleteEphemeralKey(keyReq)
		return nil, access.EphemeralAuthorizedKeyRequest{}, fmt.Errorf("ns-proxy dial for %s: %w", user, err)
	}

	innerCtx, innerCancel := context.WithTimeout(ctx, 15*time.Second)
	defer innerCancel()
	inner, err := dialInnerSSH(innerCtx, tcp, net.JoinHostPort(ip, "22"), user, signer, s.hostKeyCB)
	if err != nil {
		_ = tcp.Close()
		s.deleteEphemeralKey(keyReq)
		return nil, access.EphemeralAuthorizedKeyRequest{}, fmt.Errorf("%s: %w", user, err)
	}
	return inner, keyReq, nil
}

func (s *Server) deleteEphemeralKey(keyReq access.EphemeralAuthorizedKeyRequest) {
	dctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.keyClient.Delete(dctx, keyReq); err != nil {
		s.log.Warn("delete ephemeral authorized key", "instance_id", keyReq.InstanceID, "user", keyReq.Username, "err", err)
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
		_, _ = fmt.Fprint(rw, "invalid selection, try again\r\n")
	}
	_, _ = fmt.Fprint(rw, "too many invalid attempts; closing\r\n")
	return VM{}, false
}

// readLine reads up to a CR or LF from r. Treats CRLF and LF identically.
func readLine(r io.Reader, scratch []byte) (string, error) {
	var out []byte
	echo, _ := r.(io.Writer)
	for {
		n, err := r.Read(scratch[:1])
		if n > 0 {
			b := scratch[0]
			if b == '\n' || b == '\r' {
				if echo != nil {
					_, _ = io.WriteString(echo, "\r\n")
				}
				return string(out), nil
			}
			if b == '\b' || b == 0x7f {
				if len(out) > 0 {
					out = out[:len(out)-1]
					if echo != nil {
						_, _ = io.WriteString(echo, "\b \b")
					}
				}
				continue
			}
			out = append(out, b)
			if echo != nil {
				_, _ = echo.Write([]byte{b})
			}
		}
		if err != nil {
			return string(out), err
		}
	}
}
