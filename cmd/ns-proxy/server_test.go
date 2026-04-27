package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

// ---------- helpers ----------

// startEcho starts a TCP echo server on 127.0.0.1:0. The returned addr is the
// address clients should connect to; stop must be called to shut it down.
func startEcho(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startEcho listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) //nolint:errcheck
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// testServerHandles groups the handles returned by newTestServerFull.
type testServerHandles struct {
	srv      *Server
	sockPath string
	// closeLn closes the Unix listener immediately (non-blocking).
	// After closeLn, Serve's accept loop will see net.ErrClosed and call
	// s.wg.Wait() before returning via serveDone.
	closeLn func()
	// waitServe blocks until Serve returns (use after closeLn + all conns
	// closed). Registering as t.Cleanup ensures goroutine-leak-free tests.
	waitServe func()
	// serveErr receives the error returned by Serve exactly once (buffered,
	// capacity 1). The channel is closed after the error is placed so it is
	// safe to read via a select with a timeout.
	serveErr <-chan error
}

// newTestServerFull creates a Server + Unix-socket listener, starts Serve in a
// goroutine, and returns handles for fine-grained control. The caller is
// responsible for eventually calling closeLn (to stop accepting) and
// waitServe (to confirm the goroutine exited).
//
// macOS limits Unix socket paths to 104 chars, so we create a short temp dir
// under /tmp rather than using t.TempDir() (whose paths exceed that limit).
func newTestServerFull(t *testing.T, cidr string, maxConns int, dialTimeout time.Duration) testServerHandles {
	t.Helper()
	al, err := ParseCIDRs(cidr)
	if err != nil {
		t.Fatalf("ParseCIDRs: %v", err)
	}
	cfg := &Config{
		AllowList:     al,
		MaxConns:      maxConns,
		DialTimeout:   dialTimeout,
		ShutdownGrace: 5 * time.Second,
	}
	srv := NewServer(cfg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	dir, err := os.MkdirTemp("/tmp", "ns-proxy-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sockPath := fmt.Sprintf("%s/p.sock", dir)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}

	// errCh receives Serve's return value. The intermediary goroutine reads it,
	// puts it back (so tests can inspect it), and closes serveDone.
	rawErrCh := make(chan error, 1)
	go func() { rawErrCh <- srv.Serve(context.Background(), ln) }()

	errCh := make(chan error, 1)
	serveDone := make(chan struct{})
	go func() {
		serveResult := <-rawErrCh
		errCh <- serveResult
		close(serveDone)
	}()

	closeLn := sync.OnceFunc(func() { ln.Close() })
	waitServe := func() { <-serveDone }

	// Guarantee cleanup even if the test forgets.
	t.Cleanup(func() {
		closeLn()
		// Don't call waitServe here because Serve blocks in wg.Wait until all
		// active goroutines finish — tests that hold connections open past
		// t.Cleanup would deadlock. Leaking the goroutine at test-process exit
		// is acceptable.
	})

	return testServerHandles{
		srv:       srv,
		sockPath:  sockPath,
		closeLn:   closeLn,
		waitServe: waitServe,
		serveErr:  errCh,
	}
}

// socks5Dialer returns a proxy.Dialer that tunnels through the Unix-socket
// SOCKS5 server at sockPath.
func socks5Dialer(t *testing.T, sockPath string) proxy.Dialer {
	t.Helper()
	// golang.org/x/net/proxy.SOCKS5 needs a network+address for the proxy, but
	// we intercept the underlying Dial via a custom forwarder that actually
	// connects to the Unix socket — the "address" field is ignored.
	forward := &unixDialer{path: sockPath}
	d, err := proxy.SOCKS5("tcp", "unused", nil, forward)
	if err != nil {
		t.Fatalf("proxy.SOCKS5: %v", err)
	}
	return d
}

// unixDialer is a proxy.Dialer that ignores its address argument and always
// connects to the named Unix socket. This lets golang.org/x/net/proxy reach
// our Unix-socket SOCKS5 server.
type unixDialer struct{ path string }

func (u *unixDialer) Dial(_, _ string) (net.Conn, error) {
	return net.Dial("unix", u.path)
}

// waitActiveConns polls srv.Stats until active >= n or 2 s elapses.
func waitActiveConns(t *testing.T, srv *Server, n int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		active, _, _ := srv.Stats()
		if active >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	active, _, _ := srv.Stats()
	t.Fatalf("activeConns = %d after 2s; want >= %d", active, n)
}

// ---------- tests ----------

// TestServer_DialsAllowedDestination verifies that a SOCKS5 client can reach
// an echo server whose IP falls in the allowlist, and that data flows through.
func TestServer_DialsAllowedDestination(t *testing.T) {
	echoAddr, stopEcho := startEcho(t)
	defer stopEcho()

	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)
	defer h.closeLn()

	conn, err := socks5Dialer(t, h.sockPath).Dial("tcp", echoAddr)
	if err != nil {
		t.Fatalf("dial via SOCKS5: %v", err)
	}
	defer conn.Close()

	payload := []byte("ping-from-socks5")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(payload) {
		t.Errorf("echo mismatch: got %q, want %q", buf, payload)
	}
}

// TestServer_RejectsDeniedDestination verifies that a dial to an IP outside
// the allowlist fails at the SOCKS5 level (lib sends a non-success reply;
// golang.org/x/net/proxy surfaces it as an error).
func TestServer_RejectsDeniedDestination(t *testing.T) {
	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)
	defer h.closeLn()

	conn, err := socks5Dialer(t, h.sockPath).Dial("tcp", "8.8.8.8:53")
	if err == nil {
		conn.Close()
		t.Fatal("expected error dialing denied destination, got nil")
	}
}

// TestServer_RejectsAtMaxConns verifies that when the semaphore is full the
// server closes new connections without doing SOCKS5 negotiation, and the
// deniedDials counter increments.
func TestServer_RejectsAtMaxConns(t *testing.T) {
	echoAddr, stopEcho := startEcho(t)
	defer stopEcho()

	h := newTestServerFull(t, "127.0.0.0/8", 1, 2*time.Second)
	defer h.closeLn()

	d := socks5Dialer(t, h.sockPath)

	// Open the first connection and hold it open so the slot stays occupied.
	conn1, err := d.Dial("tcp", echoAddr)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer conn1.Close()

	// Wait for the first conn to be accounted for.
	waitActiveConns(t, h.srv, 1)

	// Second dial must fail: server closes the raw TCP conn before SOCKS5, so
	// golang.org/x/net/proxy gets EOF or a reset during the handshake.
	conn2, err := d.Dial("tcp", echoAddr)
	if err == nil {
		conn2.Close()
		t.Fatal("expected error dialing at max conns, got nil")
	}

	_, _, denied := h.srv.Stats()
	if denied < 1 {
		t.Errorf("deniedDials = %d; want >= 1", denied)
	}
}

// TestServer_StatsCountersIncrement verifies that a successful dial increments
// totalDials, and that activeConns returns to zero after the connection closes.
func TestServer_StatsCountersIncrement(t *testing.T) {
	echoAddr, stopEcho := startEcho(t)
	defer stopEcho()

	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)
	defer h.closeLn()

	conn, err := socks5Dialer(t, h.sockPath).Dial("tcp", echoAddr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	waitActiveConns(t, h.srv, 1)

	_, total, _ := h.srv.Stats()
	if total < 1 {
		t.Errorf("totalDials = %d; want >= 1", total)
	}

	conn.Close()

	// After closing, activeConns should drop back to zero.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		active, _, _ := h.srv.Stats()
		if active == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	active, _, _ := h.srv.Stats()
	t.Errorf("activeConns = %d after connection close; want 0", active)
}

// TestServer_ShutdownReturnsWhenIdle verifies that Shutdown returns immediately
// (nil) when no connections are active.
func TestServer_ShutdownReturnsWhenIdle(t *testing.T) {
	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)

	// Close listener first so Serve can exit and serveDone fires.
	h.closeLn()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := h.srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Shutdown waited on s.serveDone, so the accept loop has already exited
	// and serveErr is available without blocking.
	h.waitServe()
	select {
	case err := <-h.serveErr:
		if err != nil {
			t.Errorf("Serve returned error: %v", err)
		}
	default:
		t.Error("serveErr channel empty after waitServe — Serve goroutine did not exit")
	}
}

// TestServer_ShutdownDrainsActiveConns verifies that Shutdown waits for an
// in-flight connection and returns nil once it closes.
func TestServer_ShutdownDrainsActiveConns(t *testing.T) {
	echoAddr, stopEcho := startEcho(t)
	defer stopEcho()

	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)

	conn, err := socks5Dialer(t, h.sockPath).Dial("tcp", echoAddr)
	if err != nil {
		h.closeLn()
		t.Fatalf("dial: %v", err)
	}

	waitActiveConns(t, h.srv, 1)

	// Close the listener — stops accepting new connections.
	// Serve will call s.wg.Wait() and block until all active goroutines finish.
	// We do NOT call h.waitServe here because Serve is blocked until conn is
	// closed, which happens below.
	h.closeLn()

	// Release the connection after 100 ms so Shutdown can drain it.
	go func() {
		time.Sleep(100 * time.Millisecond)
		conn.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := h.srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	// Shutdown waited on s.serveDone, so the accept loop has already exited
	// and serveErr is available without blocking.
	h.waitServe()
	select {
	case err := <-h.serveErr:
		if err != nil {
			t.Errorf("Serve returned error: %v", err)
		}
	default:
		t.Error("serveErr channel empty after waitServe — Serve goroutine did not exit")
	}
}

// TestServer_ShutdownGraceTimeout verifies that Shutdown returns
// context.DeadlineExceeded when the context expires before all connections
// drain.
func TestServer_ShutdownGraceTimeout(t *testing.T) {
	echoAddr, stopEcho := startEcho(t)
	defer stopEcho()

	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)

	conn, err := socks5Dialer(t, h.sockPath).Dial("tcp", echoAddr)
	if err != nil {
		h.closeLn()
		t.Fatalf("dial: %v", err)
	}
	// Deliberately hold the connection open past Shutdown's timeout; close it
	// at the end so Serve's wg.Wait eventually unblocks.
	defer conn.Close()

	waitActiveConns(t, h.srv, 1)

	h.closeLn()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = h.srv.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Shutdown returned %v; want context.DeadlineExceeded", err)
	}
}
