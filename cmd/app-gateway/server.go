package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

const defaultAppTargetPort = 80

var errNotFound = errors.New("not found")

type Server struct {
	cfg      *Config
	log      *slog.Logger
	repo     appRepo
	resolver fixedIPResolver
}

type appRepo interface {
	FindByHost(ctx context.Context, host string) (*AppMapping, error)
}

func NewServer(cfg *Config, log *slog.Logger, r appRepo, resolver fixedIPResolver) *Server {
	return &Server{cfg: cfg, log: log, repo: r, resolver: resolver}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := normalizeHost(r.Host)
	if host == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

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

	rp, err := newReverseProxy(fixedIP, defaultAppTargetPort, s.cfg.NsProxySock)
	if err != nil {
		s.log.Error("proxy build failed", "host", host, "err", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	rp.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		s.log.Warn("proxy error",
			"host", req.Host,
			"instance", mapping.OpenstackID,
			"target", net.JoinHostPort(fixedIP, "80"),
			"err", err,
		)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	rp.ServeHTTP(w, r)
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
