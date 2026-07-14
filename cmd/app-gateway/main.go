package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
	rcpopenstack "github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/joho/godotenv"
)

var loadDotenv = godotenv.Load

func main() {
	if err := loadLocalEnv(); err != nil {
		fatal(".env load failed: %v", err)
	}

	cfg, err := LoadConfig(os.Getenv)
	if err != nil {
		fatal("config load failed: %v", err)
	}
	log := newLogger(cfg.LogLevel)

	db, err := database.OpenEntClient(database.Config{Driver: cfg.DBDriver, DSN: cfg.DBDSN})
	if err != nil {
		fatal("db open: %v", err)
	}
	defer func() { _ = db.Close() }()

	provider, err := rcpopenstack.NewProviderClient(rcpopenstack.ProviderConfig{
		AuthURL:     os.Getenv("OS_AUTH_URL"),
		Username:    os.Getenv("OS_USERNAME"),
		Password:    os.Getenv("OS_PASSWORD"),
		ProjectName: os.Getenv("OS_PROJECT_NAME"),
		DomainName:  os.Getenv("OS_USER_DOMAIN_NAME"),
		CFClientID:  os.Getenv("CF_ACCESS_CLIENT_ID"),
		CFSecret:    os.Getenv("CF_ACCESS_CLIENT_SECRET"),
	})
	if err != nil {
		fatal("openstack auth: %v", err)
	}
	resolver, err := newGopherResolver(provider, cfg.FixedNetworkName)
	if err != nil {
		fatal("openstack compute client: %v", err)
	}

	app, err := NewServer(cfg, log, newRepo(db), resolver)
	if err != nil {
		fatal("server build: %v", err)
	}
	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           app,
		ReadHeaderTimeout: cfg.ReadTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		log.Info("app-gateway started",
			"listen", cfg.Listen,
			"ns_proxy_sock", cfg.NsProxySock,
			"fixed_network", cfg.FixedNetworkName,
		)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http serve", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown", "err", err)
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
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "app-gateway fatal: "+format+"\n", args...)
	os.Exit(1)
}

func loadLocalEnv() error {
	if err := loadDotenv(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
