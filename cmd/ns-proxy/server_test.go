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

// startEcho는 127.0.0.1:0에 TCP echo 서버를 띄운다.
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
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

type testServerHandles struct {
	srv      *Server
	sockPath string
	// closeLn은 listener를 즉시 닫는다 (non-blocking). 닫으면 Serve의 accept
	// loop가 net.ErrClosed → wg.Wait → serveDone 순으로 정리.
	closeLn func()
	// waitServe는 Serve goroutine이 실제로 종료될 때까지 블록.
	waitServe func()
	// serveErr는 Serve의 리턴값 1회 수신 (capacity 1 buffered).
	serveErr <-chan error
}

// newTestServerFull은 Server + Unix 소켓 listener를 만들고 Serve를 goroutine으로
// 띄운다. 호출자는 closeLn(수신 중단) + waitServe(goroutine 종료 확인) 책임.
//
// macOS는 Unix 소켓 경로 길이를 104자로 제한하므로 t.TempDir() 대신 짧은 /tmp 하위 디렉터리를 사용.
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
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sockPath := fmt.Sprintf("%s/p.sock", dir)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}

	// 중간 goroutine이 Serve의 리턴값을 받아 errCh에 다시 넣고 serveDone을 닫음
	// → 테스트가 errCh를 select로 검사 가능.
	rawErrCh := make(chan error, 1)
	go func() { rawErrCh <- srv.Serve(context.Background(), ln) }()

	errCh := make(chan error, 1)
	serveDone := make(chan struct{})
	go func() {
		serveResult := <-rawErrCh
		errCh <- serveResult
		close(serveDone)
	}()

	closeLn := sync.OnceFunc(func() { _ = ln.Close() })
	waitServe := func() { <-serveDone }

	t.Cleanup(func() {
		closeLn()
		// 여기서 waitServe를 부르지 않음 — 테스트가 conn을 닫지 않은 채 t.Cleanup에
		// 진입하면 Serve의 wg.Wait이 무한 대기 → deadlock. 프로세스 종료 시 leak 허용.
	})

	return testServerHandles{
		srv:       srv,
		sockPath:  sockPath,
		closeLn:   closeLn,
		waitServe: waitServe,
		serveErr:  errCh,
	}
}

// socks5Dialer는 Unix 소켓 SOCKS5 서버를 거쳐 dial하는 proxy.Dialer를 반환.
func socks5Dialer(t *testing.T, sockPath string) proxy.Dialer {
	t.Helper()
	// proxy.SOCKS5는 network+address를 요구하지만 forward dialer로 Unix 소켓을
	// 직접 잡으므로 "address"는 무시됨.
	forward := &unixDialer{path: sockPath}
	d, err := proxy.SOCKS5("tcp", "unused", nil, forward)
	if err != nil {
		t.Fatalf("proxy.SOCKS5: %v", err)
	}
	return d
}

// unixDialer는 address 인자를 무시하고 항상 지정된 Unix 소켓에 연결.
type unixDialer struct{ path string }

func (u *unixDialer) Dial(_, _ string) (net.Conn, error) {
	return net.Dial("unix", u.path)
}

// waitActiveConns는 srv.Stats의 active가 n 이상이 될 때까지 폴링 (max 2초).
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

// SOCKS5 클라이언트가 allowlist 안의 echo 서버에 닿고 데이터가 양방향으로 흐르는지 검증.
func TestServer_DialsAllowedDestination(t *testing.T) {
	echoAddr, stopEcho := startEcho(t)
	defer stopEcho()

	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)
	defer h.closeLn()

	conn, err := socks5Dialer(t, h.sockPath).Dial("tcp", echoAddr)
	if err != nil {
		t.Fatalf("dial via SOCKS5: %v", err)
	}
	defer func() { _ = conn.Close() }()

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

// allowlist 밖 IP는 SOCKS5 레벨에서 거부 (lib이 non-success reply, proxy는 에러로 surface).
func TestServer_RejectsDeniedDestination(t *testing.T) {
	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)
	defer h.closeLn()

	conn, err := socks5Dialer(t, h.sockPath).Dial("tcp", "8.8.8.8:53")
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected error dialing denied destination, got nil")
	}
}

// sem이 가득 차면 서버는 SOCKS5 협상 없이 raw TCP를 끊고 deniedDials를 증가.
func TestServer_RejectsAtMaxConns(t *testing.T) {
	echoAddr, stopEcho := startEcho(t)
	defer stopEcho()

	h := newTestServerFull(t, "127.0.0.0/8", 1, 2*time.Second)
	defer h.closeLn()

	d := socks5Dialer(t, h.sockPath)

	// 첫 연결을 점유 상태로 유지.
	conn1, err := d.Dial("tcp", echoAddr)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	defer func() { _ = conn1.Close() }()

	waitActiveConns(t, h.srv, 1)

	// 두 번째 dial은 실패 — 서버가 SOCKS5 전에 raw TCP를 끊어 proxy가 EOF/reset을 받음.
	conn2, err := d.Dial("tcp", echoAddr)
	if err == nil {
		_ = conn2.Close()
		t.Fatal("expected error dialing at max conns, got nil")
	}

	_, _, denied := h.srv.Stats()
	if denied < 1 {
		t.Errorf("deniedDials = %d; want >= 1", denied)
	}
}

// 성공한 dial은 totalDials를 증가, 연결 종료 후 activeConns는 0으로 복귀.
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

	_ = conn.Close()

	// 종료 후 activeConns가 0으로 떨어지는지 폴링.
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

// active 연결이 없을 때 Shutdown은 즉시 nil 반환.
func TestServer_ShutdownReturnsWhenIdle(t *testing.T) {
	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)

	// listener를 먼저 닫아야 Serve가 종료되고 serveDone이 fire됨.
	h.closeLn()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := h.srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Shutdown이 serveDone을 기다렸으므로 accept loop는 이미 종료, serveErr는 즉시 가용.
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

// in-flight 연결이 있으면 Shutdown은 그것이 닫힐 때까지 대기 후 nil 반환.
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

	// listener를 닫으면 Serve는 wg.Wait로 진입해 active goroutine을 기다림.
	// conn이 아래에서 닫히기 전까지 Serve가 블록되므로 여기서는 waitServe를 호출하지 않음.
	h.closeLn()

	// 100ms 후 conn 해제 → Shutdown이 drain 가능하도록.
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = conn.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := h.srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

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

// drain 완료 전 ctx가 만료되면 Shutdown은 context.DeadlineExceeded 반환.
func TestServer_ShutdownGraceTimeout(t *testing.T) {
	echoAddr, stopEcho := startEcho(t)
	defer stopEcho()

	h := newTestServerFull(t, "127.0.0.0/8", 10, 2*time.Second)

	conn, err := socks5Dialer(t, h.sockPath).Dial("tcp", echoAddr)
	if err != nil {
		h.closeLn()
		t.Fatalf("dial: %v", err)
	}
	// Shutdown 타임아웃 이후까지 conn을 의도적으로 유지; 끝에서 닫아 Serve의 wg.Wait이 풀리게.
	defer func() { _ = conn.Close() }()

	waitActiveConns(t, h.srv, 1)

	h.closeLn()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = h.srv.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Shutdown returned %v; want context.DeadlineExceeded", err)
	}
}
