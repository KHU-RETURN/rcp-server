package access

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	webConsoleKeepaliveInterval = 30 * time.Second
	webConsoleProbeTimeout      = 10 * time.Second
	webConsoleIdleTimeout       = 30 * time.Minute
	webConsoleMaxLifetime       = 12 * time.Hour
)

var (
	errConsoleIdle     = errors.New("console idle timeout exceeded")
	errConsoleLifetime = errors.New("console max lifetime exceeded")
)

// consoleProbe checks one liveness dimension of an idle console (e.g. the
// WebSocket peer or the SSH transport) without generating terminal traffic.
type consoleProbe func(context.Context) error

// consoleLiveness decides when an interactive console must be torn down:
// idle too long, alive past the absolute cap, or a liveness probe failing.
// Idle detection matters because a dead peer is otherwise only noticed when
// terminal output forces a write.
type consoleLiveness struct {
	started     time.Time
	idleTimeout time.Duration
	maxLifetime time.Duration
	probes      []consoleProbe
	lastActive  atomic.Int64 // unix nanoseconds
}

func newConsoleLiveness(now time.Time, idleTimeout, maxLifetime time.Duration, probes ...consoleProbe) *consoleLiveness {
	l := &consoleLiveness{
		started:     now,
		idleTimeout: idleTimeout,
		maxLifetime: maxLifetime,
		probes:      probes,
	}
	l.touch(now)
	return l
}

func (l *consoleLiveness) touch(now time.Time) {
	l.lastActive.Store(now.UnixNano())
}

func (l *consoleLiveness) check(ctx context.Context, now time.Time) error {
	if now.Sub(l.started) >= l.maxLifetime {
		return errConsoleLifetime
	}
	if now.Sub(time.Unix(0, l.lastActive.Load())) >= l.idleTimeout {
		return errConsoleIdle
	}
	for _, probe := range l.probes {
		if err := probe(ctx); err != nil {
			return fmt.Errorf("console liveness probe failed: %w", err)
		}
	}
	return nil
}

// trackedConsole stamps liveness on every successful client read/write so the
// idle timeout only fires on genuinely quiet sessions.
type trackedConsole struct {
	io.ReadWriteCloser
	liveness *consoleLiveness
}

func (t *trackedConsole) Read(p []byte) (int, error) {
	n, err := t.ReadWriteCloser.Read(p)
	if n > 0 {
		t.liveness.touch(time.Now())
	}
	return n, err
}

func (t *trackedConsole) Write(p []byte) (int, error) {
	n, err := t.ReadWriteCloser.Write(p)
	if n > 0 {
		t.liveness.touch(time.Now())
	}
	return n, err
}

// watchConsole periodically checks liveness and cancels the relay with the
// failure as cause, so teardown logs say why the session ended.
func watchConsole(ctx context.Context, cancel context.CancelCauseFunc, interval time.Duration, liveness *consoleLiveness) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probeCtx, probeCancel := context.WithTimeout(ctx, webConsoleProbeTimeout)
			err := liveness.check(probeCtx, time.Now())
			probeCancel()
			if err != nil {
				cancel(err)
				return
			}
		}
	}
}

// sshKeepaliveProbe detects a dead VM or proxy path on an idle session, like
// OpenSSH's ServerAliveInterval. SendRequest has no context support, so the
// reply is awaited in a goroutine; on timeout the goroutine is abandoned and
// reclaimed when the connection closes at teardown.
func sshKeepaliveProbe(conn ssh.Conn) consoleProbe {
	return func(ctx context.Context) error {
		done := make(chan error, 1)
		go func() {
			_, _, err := conn.SendRequest("keepalive@openssh.com", true, nil)
			done <- err
		}()
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
