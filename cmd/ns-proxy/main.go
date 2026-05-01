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
	_ = os.Remove(cfg.SockPath) // 이전 크래시로 남은 stale 소켓 허용 — 아래에서 재생성

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

	statsCtx, statsCancel := context.WithCancel(ctx)
	defer statsCancel()
	go statsLoop(statsCtx, srv, log, statsInterval)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx, ln) }()

	// Serve가 첫 select에서 이미 끝났는지 — 끝났다면 채널이 비어 있어 두 번째 대기를 건너뜀.
	var serveExited bool
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		serveExited = true
		if err != nil {
			log.Error("serve aborted", "err", err)
			os.Exit(1)
		}
	}

	// 신호 컨텍스트가 이미 cancel된 상태라 두 번째 SIGTERM/SIGINT는 흡수됨.
	// 기본 핸들러를 복원해 두 번째 시그널이 즉시 프로세스를 죽이도록.
	stopSignals()

	active, _, _ := srv.Stats()
	log.Info("shutting down", "active_conns", active)

	_ = ln.Close()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("grace timeout, forcing close", "err", err)
	} else {
		log.Info("drained gracefully")
	}

	if !serveExited {
		select {
		case err := <-serveErr:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				log.Warn("serve returned error", "err", err)
			}
		case <-time.After(2 * time.Second):
			log.Warn("serve goroutine did not return after shutdown")
		}
	}
}

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
