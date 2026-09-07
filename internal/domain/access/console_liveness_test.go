package access

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"
)

func TestConsoleLivenessCheck(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	probeErr := errors.New("probe boom")

	tests := []struct {
		name    string
		probes  []consoleProbe
		now     time.Time
		wantErr error
	}{
		{
			name: "passes while active and within lifetime",
			now:  t0.Add(time.Minute),
		},
		{
			name:    "fails after idle timeout",
			now:     t0.Add(webConsoleIdleTimeout + time.Second),
			wantErr: errConsoleIdle,
		},
		{
			name:    "fails after max lifetime",
			now:     t0.Add(webConsoleMaxLifetime + time.Second),
			wantErr: errConsoleLifetime,
		},
		{
			name:    "fails when a probe fails",
			probes:  []consoleProbe{func(context.Context) error { return probeErr }},
			now:     t0.Add(time.Minute),
			wantErr: probeErr,
		},
		{
			name: "passes when probes succeed",
			probes: []consoleProbe{
				func(context.Context) error { return nil },
				func(context.Context) error { return nil },
			},
			now: t0.Add(time.Minute),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newConsoleLiveness(t0, webConsoleIdleTimeout, webConsoleMaxLifetime, tt.probes...)
			err := l.check(context.Background(), tt.now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("check() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestConsoleLivenessTouchResetsIdle(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	l := newConsoleLiveness(t0, webConsoleIdleTimeout, webConsoleMaxLifetime)

	stale := t0.Add(webConsoleIdleTimeout + time.Second)
	if err := l.check(context.Background(), stale); !errors.Is(err, errConsoleIdle) {
		t.Fatalf("check() before touch = %v, want %v", err, errConsoleIdle)
	}

	l.touch(stale)
	if err := l.check(context.Background(), stale); err != nil {
		t.Fatalf("check() after touch = %v, want nil", err)
	}
}

type fakeRWC struct {
	readData []byte
	closed   bool
}

func (f *fakeRWC) Read(p []byte) (int, error) {
	if len(f.readData) == 0 {
		return 0, io.EOF
	}
	n := copy(p, f.readData)
	f.readData = f.readData[n:]
	return n, nil
}

func (f *fakeRWC) Write(p []byte) (int, error) { return len(p), nil }
func (f *fakeRWC) Close() error                { f.closed = true; return nil }

func TestTrackedConsoleStampsActivity(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	idleAt := t0.Add(webConsoleIdleTimeout + time.Second)

	t.Run("read resets idle clock", func(t *testing.T) {
		l := newConsoleLiveness(t0, webConsoleIdleTimeout, webConsoleMaxLifetime)
		tracked := &trackedConsole{ReadWriteCloser: &fakeRWC{readData: []byte("ls\n")}, liveness: l}

		buf := make([]byte, 8)
		if _, err := tracked.Read(buf); err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if err := l.check(context.Background(), idleAt); errors.Is(err, errConsoleIdle) {
			t.Fatal("check() reported idle right after a read")
		}
	})

	t.Run("write resets idle clock", func(t *testing.T) {
		l := newConsoleLiveness(t0, webConsoleIdleTimeout, webConsoleMaxLifetime)
		tracked := &trackedConsole{ReadWriteCloser: &fakeRWC{}, liveness: l}

		if _, err := tracked.Write([]byte("output")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := l.check(context.Background(), idleAt); errors.Is(err, errConsoleIdle) {
			t.Fatal("check() reported idle right after a write")
		}
	})
}

func TestWatchConsoleCancelsWithCause(t *testing.T) {
	probeErr := errors.New("dead transport")
	l := newConsoleLiveness(time.Now(), webConsoleIdleTimeout, webConsoleMaxLifetime,
		func(context.Context) error { return probeErr })

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	go watchConsole(ctx, cancel, time.Millisecond, l)

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("watchConsole did not cancel on probe failure")
	}
	if cause := context.Cause(ctx); !errors.Is(cause, probeErr) {
		t.Fatalf("context cause = %v, want %v", cause, probeErr)
	}
}

func TestWatchConsoleStopsWhenContextDone(t *testing.T) {
	l := newConsoleLiveness(time.Now(), webConsoleIdleTimeout, webConsoleMaxLifetime)

	ctx, cancel := context.WithCancelCause(context.Background())
	stopped := make(chan struct{})
	go func() {
		watchConsole(ctx, cancel, time.Millisecond, l)
		close(stopped)
	}()

	cancel(nil)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("watchConsole did not stop after context cancellation")
	}
}

type fakeSSHConn struct {
	ssh.Conn
	sendRequest func(name string, wantReply bool, payload []byte) (bool, []byte, error)
}

func (f *fakeSSHConn) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	return f.sendRequest(name, wantReply, payload)
}

func TestSSHKeepaliveProbe(t *testing.T) {
	t.Run("sends openssh keepalive and succeeds", func(t *testing.T) {
		var gotName string
		var gotWantReply bool
		conn := &fakeSSHConn{sendRequest: func(name string, wantReply bool, _ []byte) (bool, []byte, error) {
			gotName, gotWantReply = name, wantReply
			return false, nil, nil
		}}

		if err := sshKeepaliveProbe(conn)(context.Background()); err != nil {
			t.Fatalf("probe error = %v, want nil", err)
		}
		if gotName != "keepalive@openssh.com" || !gotWantReply {
			t.Fatalf("SendRequest(%q, %v), want (keepalive@openssh.com, true)", gotName, gotWantReply)
		}
	})

	t.Run("propagates transport error", func(t *testing.T) {
		sendErr := errors.New("connection lost")
		conn := &fakeSSHConn{sendRequest: func(string, bool, []byte) (bool, []byte, error) {
			return false, nil, sendErr
		}}

		if err := sshKeepaliveProbe(conn)(context.Background()); !errors.Is(err, sendErr) {
			t.Fatalf("probe error = %v, want %v", err, sendErr)
		}
	})

	t.Run("honors context when request hangs", func(t *testing.T) {
		block := make(chan struct{})
		defer close(block)
		conn := &fakeSSHConn{sendRequest: func(string, bool, []byte) (bool, []byte, error) {
			<-block
			return false, nil, nil
		}}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := sshKeepaliveProbe(conn)(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("probe error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestWebSocketConsolePing(t *testing.T) {
	consoles := make(chan *webSocketConsole, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		consoles <- newWebSocketConsole(conn)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("websocket.Dial() error = %v", err)
	}
	defer func() { _ = clientConn.CloseNow() }()
	// Both peers must be reading for ping/pong control frames to be processed.
	go func() {
		for {
			if _, _, err := clientConn.Read(context.Background()); err != nil {
				return
			}
		}
	}()

	console := <-consoles
	defer func() { _ = console.Close() }()
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := console.Read(buf); err != nil {
				return
			}
		}
	}()

	if err := console.Ping(ctx); err != nil {
		t.Fatalf("Ping() over live connection = %v, want nil", err)
	}

	_ = clientConn.CloseNow()
	pingCtx, pingCancel := context.WithTimeout(context.Background(), time.Second)
	defer pingCancel()
	if err := console.Ping(pingCtx); err == nil {
		t.Fatal("Ping() over closed connection = nil, want error")
	}
}
