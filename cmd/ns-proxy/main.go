package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const statsInterval = time.Minute

func main() {
	cfg, err := LoadConfig(os.Getenv)
	if err != nil {
		fatal("config load failed: %v", err)
	}

	log := newLogger(cfg.LogLevel)

	if err := os.MkdirAll(filepath.Dir(cfg.SockPath), 0o750); err != nil {
		fatal("mkdir socket dir: %v", err)
	}
	_ = os.Remove(cfg.SockPath) // tolerate stale socket from prior crash; recreated below

	ln, err := net.Listen("unix", cfg.SockPath)
	if err != nil {
		fatal("listen %s: %v", cfg.SockPath, err)
	}
	if err := os.Chmod(cfg.SockPath, 0o660); err != nil {
		fatal("chmod %s: %v", cfg.SockPath, err)
	}

	srv := NewServer(cfg, log)

	log.Info("ns-proxy started",
		"sock", cfg.SockPath,
		"max_conns", cfg.MaxConns,
		"dial_timeout", cfg.DialTimeout,
		"shutdown_grace", cfg.ShutdownGrace,
		"log_level", cfg.LogLevel,
	)

	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)

	// Stats logger: emit once at startup + every statsInterval until ctx cancels.
	statsCtx, statsCancel := context.WithCancel(ctx)
	defer statsCancel()
	go statsLoop(statsCtx, srv, log, statsInterval)

	// Serve in a goroutine so we can react to signals.
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx, ln) }()

	select {
	case <-ctx.Done():
		// Signal received, fall through to shutdown.
	case err := <-serveErr:
		if err != nil {
			log.Error("serve aborted", "err", err)
			os.Exit(1)
		}
		// Serve returned nil unexpectedly (listener closed externally?). Still graceful.
	}

	// Restore default signal behavior so a second SIGTERM/SIGINT kills immediately
	// instead of being absorbed by the (already-cancelled) signal context.
	stopSignals()

	active, _, _ := srv.Stats()
	log.Info("shutting down", "active_conns", active)

	// Stop accepting new conns; drain in-flight.
	_ = ln.Close()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("grace timeout, forcing close", "err", err)
	} else {
		log.Info("drained gracefully")
	}

	// Wait for Serve goroutine to publish its final return.
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.Warn("serve returned error", "err", err)
		}
	case <-time.After(2 * time.Second):
		log.Warn("serve goroutine did not return after shutdown")
	}
}

// statsLoop logs counter snapshots once at startup and then on every interval
// until ctx is cancelled.
func statsLoop(ctx context.Context, srv *Server, log *slog.Logger, interval time.Duration) {
	emit := func() {
		active, total, denied := srv.Stats()
		log.Info("stats", "active", active, "total_dials", total, "denied", denied)
	}
	emit()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			emit()
		}
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ns-proxy fatal: "+format+"\n", args...)
	os.Exit(1)
}
