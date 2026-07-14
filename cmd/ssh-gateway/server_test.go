package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type scriptedReadWriter struct {
	in  *bytes.Reader
	out bytes.Buffer
}

func newScriptedReadWriter(input string) *scriptedReadWriter {
	return &scriptedReadWriter{in: bytes.NewReader([]byte(input))}
}

func (rw *scriptedReadWriter) Read(p []byte) (int, error) {
	return rw.in.Read(p)
}

func (rw *scriptedReadWriter) Write(p []byte) (int, error) {
	return rw.out.Write(p)
}

func TestReadLineEchoesInputToTerminal(t *testing.T) {
	rw := newScriptedReadWriter("2\r")

	got, err := readLine(rw, make([]byte, 64))
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "2" {
		t.Fatalf("got %q", got)
	}
	if rw.out.String() != "2\r\n" {
		t.Fatalf("echo got %q", rw.out.String())
	}
}

func TestReadLineEchoesBackspace(t *testing.T) {
	rw := newScriptedReadWriter("12\x7f3\n")

	got, err := readLine(rw, make([]byte, 64))
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "13" {
		t.Fatalf("got %q", got)
	}
	if rw.out.String() != "12\b \b3\r\n" {
		t.Fatalf("echo got %q", rw.out.String())
	}
}

// newTestSSHGatewayServer builds a Server with just enough state for Serve's
// accept loop; handle() will block on the SSH handshake (never completing it
// in these tests) rather than error out immediately, which is enough to hold
// a semaphore slot open.
func newTestSSHGatewayServer(t *testing.T, maxConns int) *Server {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	sc := &ssh.ServerConfig{
		// A configured auth method is required or NewServerConn rejects the
		// conn before any network I/O; the fake client below never speaks
		// the protocol, so this callback is never actually invoked.
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			return nil, errors.New("unused")
		},
	}
	sc.AddHostKey(signer)

	return &Server{
		cfg:       &Config{NonceTTL: 5 * time.Minute},
		log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		sshConfig: sc,
		sem:       make(chan struct{}, maxConns),
	}
}

func waitSSHGatewayActiveConns(t *testing.T, srv *Server, n int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		active, _ := srv.Stats()
		if active >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	active, _ := srv.Stats()
	t.Fatalf("activeConns = %d after 2s; want >= %d", active, n)
}

// sem이 가득 차면 Serve는 SSH 핸드셰이크 전에 raw TCP를 끊고 deniedConns를 증가.
func TestServer_RejectsAtMaxConns(t *testing.T) {
	srv := newTestSSHGatewayServer(t, 1)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx, ln) }()

	// First connection: don't speak SSH, just hold the slot open.
	conn1, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer func() { _ = conn1.Close() }()

	waitSSHGatewayActiveConns(t, srv, 1)

	// Second connection must be closed immediately — sem is full.
	conn2, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	defer func() { _ = conn2.Close() }()

	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn2.Read(buf); err == nil {
		t.Fatal("expected second connection to be closed by server, got data")
	}

	if _, denied := srv.Stats(); denied < 1 {
		t.Errorf("deniedConns = %d; want >= 1", denied)
	}
}
