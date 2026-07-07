package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultAppTargetPort = 80
	hostCacheTTL         = 30 * time.Second
)

var errNotFound = errors.New("not found")

// backend is a resolved proxy target for a host.
type backend struct {
	ip          string
	openstackID string
}

type hostCacheEntry struct {
	backend backend
	expires time.Time
}

type Server struct {
	cfg       *Config
	log       *slog.Logger
	repo      appRepo
	resolver  fixedIPResolver
	transport http.RoundTripper

	mu sync.Mutex
	// hostCache maps host -> resolved backend with a TTL so the hot path
	// skips the per-request DB lookup + OpenStack servers.Get.
	// ponytail: no LRU/size cap — entries are bounded by the number of app
	// mappings in the DB, which is tiny at this scale.
	hostCache map[string]hostCacheEntry
}

type appRepo interface {
	FindByHost(ctx context.Context, host string) (*AppMapping, error)
}

func NewServer(cfg *Config, log *slog.Logger, r appRepo, resolver fixedIPResolver) (*Server, error) {
	transport, err := transportViaNSProxy(cfg.NsProxySock)
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:       cfg,
		log:       log,
		repo:      r,
		resolver:  resolver,
		transport: transport,
		hostCache: make(map[string]hostCacheEntry),
	}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := normalizeHost(r.Host)
	if host == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	be, ok := s.cachedBackend(host)
	if !ok {
		mapping, err := s.repo.FindByHost(r.Context(), host)
		if err != nil {
			if errors.Is(err, errNotFound) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			s.log.Error("app lookup failed", "host", host, "err", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		fixedIP, err := s.resolver.ResolveFixedIPv4(ctx, mapping.OpenstackID)
		if err != nil {
			s.log.Warn("fixed ip resolve failed", "host", host, "instance", mapping.OpenstackID, "err", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		be = backend{ip: fixedIP, openstackID: mapping.OpenstackID}
		s.storeBackend(host, be)
	}

	rp := newReverseProxy(be.ip, defaultAppTargetPort, s.transport)
	rp.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		s.log.Warn("proxy error",
			"host", req.Host,
			"instance", be.openstackID,
			"target", net.JoinHostPort(be.ip, "80"),
			"err", err,
		)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	rp.ServeHTTP(w, r)
}

func (s *Server) cachedBackend(host string) (backend, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.hostCache[host]
	if !ok || time.Now().After(e.expires) {
		return backend{}, false
	}
	return e.backend, true
}

func (s *Server) storeBackend(host string, be backend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostCache[host] = hostCacheEntry{backend: be, expires: time.Now().Add(hostCacheTTL)}
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return strings.ToLower(strings.TrimSpace(h))
	}
	return host
}
