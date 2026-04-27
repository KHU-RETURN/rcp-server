package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	socks5 "github.com/things-go/go-socks5"
)

// shutdownPollInterval is how often Shutdown checks activeConns while waiting
// for in-flight handlers to drain.
const shutdownPollInterval = 50 * time.Millisecond

// Server wraps a *socks5.Server with a semaphore (to cap concurrent
// connections) and a graceful-shutdown procedure. The caller owns the listener
// lifecycle: close the listener to stop the accept loop, then call Shutdown to
// drain in-flight connections.
type Server struct {
	cfg         *Config
	log         *slog.Logger
	socksServer *socks5.Server
	sem         chan struct{}
	wg          sync.WaitGroup

	serveDone chan struct{} // closed when Serve returns
	serveOnce sync.Once

	activeConns int64 // updated with sync/atomic
	totalDials  int64 // updated with sync/atomic
	deniedDials int64 // updated with sync/atomic
}

// NewServer constructs a Server. cfg.MaxConns is guaranteed > 0 by Config
// validation, so sem always has positive capacity.
func NewServer(cfg *Config, log *slog.Logger) *Server {
	return &Server{
		cfg:         cfg,
		log:         log,
		socksServer: NewSOCKS5Server(cfg, log),
		sem:         make(chan struct{}, cfg.MaxConns),
		serveDone:   make(chan struct{}),
	}
}

// Serve runs the accept loop on ln. ctx is accepted for caller-side API
// symmetry but is intentionally NOT consulted here; cancel the loop by closing
// ln (caller's responsibility). This keeps the lib's ServeConn semantics
// unchanged and avoids a second cancellation channel.
//
// Serve returns nil when the listener is closed (graceful shutdown path). Any
// other accept failure triggers a logged backoff-retry so that transient OS
// errors (EMFILE, EAGAIN) do not kill the daemon.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	defer s.serveOnce.Do(func() { close(s.serveDone) })

	var tempDelay time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				// Listener was closed by the Shutdown caller. Drain active conns
				// then signal a clean exit to the caller.
				s.wg.Wait()
				return nil
			}
			// Backoff and retry on transient errors. EMFILE, EAGAIN, etc.
			// Permanent errors (e.g. listener gone for any reason other than ErrClosed)
			// will show up as a continuous error stream that the operator sees in logs.
			if tempDelay == 0 {
				tempDelay = 10 * time.Millisecond
			} else {
				tempDelay *= 2
			}
			if tempDelay > time.Second {
				tempDelay = time.Second
			}
			s.log.Warn("accept transient error, retrying", "err", err, "delay", tempDelay)
			time.Sleep(tempDelay)
			continue
		}
		tempDelay = 0

		// Non-blocking semaphore acquire.
		select {
		case s.sem <- struct{}{}:
			atomic.AddInt64(&s.totalDials, 1)
			atomic.AddInt64(&s.activeConns, 1)
			s.wg.Add(1)
			go func() {
				// LIFO order will be: <-s.sem (first) → activeConns-- (middle) → wg.Done (last).
				// wg.Done MUST be last so Shutdown's wg.Wait sees all in-flight goroutines
				// even after activeConns reaches zero. The sem must release before activeConns--
				// so capacity is available the instant Stats reports the slot is free.
				defer s.wg.Done()                         // registered 1st → runs LAST
				defer atomic.AddInt64(&s.activeConns, -1) // registered 2nd
				defer func() { <-s.sem }()                // registered 3rd → runs FIRST
				// TODO(P2): the lib has no handshake-only read deadline hook. A slow/silent
				// client that completes TCP but never sends a SOCKS5 greeting can pin a sem
				// slot indefinitely. Mitigation options under consideration:
				//   - cfg.HandshakeTimeout + conn.SetReadDeadline before ServeConn (but lib
				//     doesn't clear deadline post-handshake, breaking long-lived sessions)
				//   - timeoutConn wrapper that clears the deadline after the first ~10 bytes
				//   - patch lib to expose a handshake hook
				// Defer to a separate issue; do not attempt here.
				if err := s.socksServer.ServeConn(conn); err != nil {
					s.log.Info("serve conn closed", "err", err)
				}
			}()
		default:
			// Semaphore full — we cannot politely communicate a "try again" to
			// a SOCKS5 client without completing the handshake first. Closing
			// the TCP connection is the cleanest max-conns signal; the operator
			// sees the denied counter increment in logs/metrics.
			atomic.AddInt64(&s.deniedDials, 1)
			s.log.Warn("rejected: max conns reached")
			_ = conn.Close()
		}
	}
}

// Shutdown waits for the accept loop to exit (caller MUST close the listener
// first; calling Shutdown without closing the listener will block forever or
// until ctx fires) and then for active handler goroutines to drain. Returns
// nil on full drain, ctx.Err() if the context fires first.
func (s *Server) Shutdown(ctx context.Context) error {
	// 1. Wait for accept loop to fully exit (caller closed listener already).
	select {
	case <-s.serveDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	// 2. Drain in-flight handlers.
	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	for {
		if atomic.LoadInt64(&s.activeConns) == 0 {
			s.wg.Wait()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Stats returns a snapshot of the three connection counters. Values are
// read atomically and may be slightly stale relative to each other, which is
// acceptable for diagnostic/logging purposes.
func (s *Server) Stats() (active, total, denied int64) {
	return atomic.LoadInt64(&s.activeConns),
		atomic.LoadInt64(&s.totalDials),
		atomic.LoadInt64(&s.deniedDials)
}
