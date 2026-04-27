package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	socks5 "github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

// cidrRuleSet allows a destination only when its IPv4 address falls in the
// configured Allowlist. Non-IPv4 destinations are always rejected as
// defense-in-depth (the Resolver should already strip IPv6, but a malicious or
// misconfigured client could send a literal IPv6 ATYP).
type cidrRuleSet struct {
	allow *Allowlist
}

// Allow implements socks5.RuleSet. Spec §3 limits supported commands to CONNECT
// only; BIND and UDP ASSOCIATE are rejected here before the IP/CIDR check.
// Without this guard, a ASSOCIATE handshake whose hint IP passes the allowlist
// would let the client relay UDP to arbitrary destinations — the RuleSet is only
// called once at handshake time, not per-datagram.
//
// Note: req.DestAddr.IP is 16-byte (IPv4-in-IPv6) for IPv4 ATYP and 4-byte for
// domain ATYP. The .To4() call normalises both forms before the allowlist check.
func (r *cidrRuleSet) Allow(ctx context.Context, req *socks5.Request) (context.Context, bool) {
	if req.Command != statute.CommandConnect {
		return ctx, false
	}
	ip := req.DestAddr.IP
	if ip == nil || ip.To4() == nil {
		return ctx, false
	}
	return ctx, r.allow.Contains(ip.To4())
}

// ipv4OnlyResolver resolves a hostname to its first IPv4 address. IPv6-only
// hosts return an error, which the lib translates to SOCKS5 reply
// RepHostUnreachable — it does not silently pick an IPv6 address.
type ipv4OnlyResolver struct{}

// Resolve implements socks5.NameResolver.
func (ipv4OnlyResolver) Resolve(ctx context.Context, name string) (context.Context, net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", name)
	if err != nil {
		return ctx, nil, fmt.Errorf("resolve %s: %w", name, err)
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return ctx, v4, nil
		}
	}
	return ctx, nil, fmt.Errorf("no IPv4 address for %s", name)
}

// slogLogger adapts a *slog.Logger to the things-go/go-socks5 Logger
// interface (a single Errorf method).
type slogLogger struct {
	log *slog.Logger
}

// Errorf implements socks5.Logger. Errorf forwards lib log lines to slog. We
// pre-format with fmt.Sprintf because the lib uses printf-style varargs, not
// slog-style key/value pairs — collapsing to a single message is the only
// sensible adapter shape here.
func (l *slogLogger) Errorf(format string, args ...any) {
	l.log.Error(fmt.Sprintf(format, args...))
}

// NewSOCKS5Server wires the lib server with our adapters. Only No-Auth is
// accepted; Unix socket file permissions are the real authentication boundary.
// WithDial injects cfg.DialTimeout so that CONNECT to black-holed-but-allowed
// destinations fails within the configured bound instead of hanging for the
// kernel default (~75 s).
func NewSOCKS5Server(cfg *Config, log *slog.Logger) *socks5.Server {
	dialer := &net.Dialer{Timeout: cfg.DialTimeout}
	return socks5.NewServer(
		socks5.WithAuthMethods([]socks5.Authenticator{socks5.NoAuthAuthenticator{}}),
		socks5.WithResolver(ipv4OnlyResolver{}),
		socks5.WithRule(&cidrRuleSet{allow: cfg.AllowList}),
		socks5.WithLogger(&slogLogger{log: log}),
		socks5.WithDial(dialer.DialContext),
	)
}
