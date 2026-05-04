package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KHU-RETURN/rcp-server/internal/infrastructure/database"
	rcpopenstack "github.com/KHU-RETURN/rcp-server/internal/infrastructure/openstack"
	"github.com/gophercloud/gophercloud"
	goopenstack "github.com/gophercloud/gophercloud/openstack"
	gccompute "github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

func main() {
	cfg, err := LoadConfig(os.Getenv)
	if err != nil {
		fatal("config load failed: %v", err)
	}
	log := newLogger(cfg.LogLevel)

	dbDriver := strings.TrimSpace(os.Getenv("DB_DRIVER"))
	if dbDriver == "" {
		dbDriver = "sqlite3"
	}
	dbDSN := strings.TrimSpace(os.Getenv("DB_DSN"))
	if dbDSN == "" {
		dbDSN = "file:" + cfg.DBPath + "?cache=shared&_pragma=foreign_keys(1)"
	}
	db, err := database.NewEntClient(database.Config{Driver: dbDriver, DSN: dbDSN})
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
	resolver := newGopherResolver(provider)

	store := newSessionStore(cfg.NonceTTL)
	r := newRepo(db)

	notifyLn, err := listenNotifySocket(cfg.NotifySock, log)
	if err != nil {
		fatal("notify listen: %v", err)
	}

	srv, err := NewServer(cfg, log, store, r, resolver)
	if err != nil {
		fatal("ssh server build: %v", err)
	}

	sshLn, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		fatal("ssh listen %s: %v", cfg.Listen, err)
	}
	log.Info("ssh-gateway started",
		"listen", cfg.Listen,
		"notify_sock", cfg.NotifySock,
		"ns_proxy_sock", cfg.NsProxySock,
		"nonce_ttl", cfg.NonceTTL,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		notifyHandler := newNotifyHandler(store, []byte(cfg.NotifySecret))
		if err := runNotifyServer(ctx, notifyLn, notifyHandler, log); err != nil {
			log.Error("notify server", "err", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := srv.Serve(ctx, sshLn); err != nil {
			log.Error("ssh serve", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	_ = sshLn.Close()
	_ = notifyLn.Close()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		log.Info("drained")
	case <-time.After(30 * time.Second):
		log.Warn("shutdown timeout, forcing exit")
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
	fmt.Fprintf(os.Stderr, "ssh-gateway fatal: "+format+"\n", args...)
	os.Exit(1)
}

// gopherResolver implements vmAddressResolver against the live OpenStack API.
// It picks the first IPv4 address found in any network of the server.
type gopherResolver struct {
	provider *gophercloud.ProviderClient
}

func newGopherResolver(p *gophercloud.ProviderClient) *gopherResolver { return &gopherResolver{provider: p} }

func (g *gopherResolver) ResolveFixedIPv4(_ context.Context, openstackID string) (string, error) {
	c, err := goopenstack.NewComputeV2(g.provider, gophercloud.EndpointOpts{Region: "RegionOne"})
	if err != nil {
		return "", err
	}
	srv, err := gccompute.Get(c, openstackID).Extract()
	if err != nil {
		return "", err
	}
	for _, raw := range srv.Addresses {
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if v, _ := m["version"].(float64); v != 4 {
				continue
			}
			if addr, _ := m["addr"].(string); addr != "" {
				return addr, nil
			}
		}
	}
	return "", errors.New("no IPv4 address on server")
}
