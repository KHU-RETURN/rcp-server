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

	store := newSessionStore(cfg.NonceTTL, cfg.MaxPendingSessions)
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
		"max_pending_sessions", cfg.MaxPendingSessions,
		"fixed_network", cfg.FixedNetworkName,
		"vm_users", strings.Join(cfg.VMUsers, ","),
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

func loadLocalEnv() error {
	if err := loadDotenv(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// gopherResolver implements vmAddressResolver against the live OpenStack API.
// It picks the first IPv4 address found in any network of the server.
type gopherResolver struct {
	compute      *gophercloud.ServiceClient
	fixedNetwork string
}

func newGopherResolver(p *gophercloud.ProviderClient, fixedNetwork string) (*gopherResolver, error) {
	c, err := goopenstack.NewComputeV2(p, gophercloud.EndpointOpts{Region: "RegionOne"})
	if err != nil {
		return nil, err
	}
	return &gopherResolver{compute: c, fixedNetwork: strings.TrimSpace(fixedNetwork)}, nil
}

func (g *gopherResolver) ResolveVM(_ context.Context, openstackID string) (VMRuntime, error) {
	srv, err := gccompute.Get(g.compute, openstackID).Extract()
	if err != nil {
		return VMRuntime{}, err
	}
	fixedIP, err := fixedIPv4FromAddresses(srv.Addresses, g.fixedNetwork)
	if err != nil {
		return VMRuntime{Status: srv.Status}, nil
	}
	return VMRuntime{Status: srv.Status, FixedIPv4: fixedIP}, nil
}

func fixedIPv4FromAddresses(addresses map[string]any, fixedNetwork string) (string, error) {
	fixedNetwork = strings.TrimSpace(fixedNetwork)
	var fixedIPs []string
	for network, raw := range addresses {
		if fixedNetwork != "" && network != fixedNetwork {
			continue
		}
		entries, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			m, ok := e.(map[string]any)
			if !ok || !isIPv4Version(m["version"]) {
				continue
			}
			if ipType, _ := m["OS-EXT-IPS:type"].(string); strings.TrimSpace(ipType) != "fixed" {
				continue
			}
			if addr, _ := m["addr"].(string); strings.TrimSpace(addr) != "" {
				fixedIPs = append(fixedIPs, strings.TrimSpace(addr))
			}
		}
	}
	switch len(fixedIPs) {
	case 0:
		if fixedNetwork != "" {
			return "", fmt.Errorf("no fixed IPv4 address on network %q", fixedNetwork)
		}
		return "", errors.New("no fixed IPv4 address on server")
	case 1:
		return fixedIPs[0], nil
	default:
		return "", fmt.Errorf("multiple fixed IPv4 addresses found; set RCP_SSH_GW_FIXED_NETWORK")
	}
}

func isIPv4Version(v any) bool {
	switch n := v.(type) {
	case float64:
		return n == 4
	case int:
		return n == 4
	case string:
		return n == "4"
	default:
		return false
	}
}
