package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	socks5 "github.com/things-go/go-socks5"
	"github.com/things-go/go-socks5/statute"
)

// cidrRuleSet은 Allowlist에 포함된 IPv4 목적지만 허용.
// IPv6는 항상 거부 — resolver가 1차로 막지만 literal IPv6 ATYP 우회 대비.
type cidrRuleSet struct {
	allow *Allowlist
}

// Allow는 IP/CIDR 검사 전에 CONNECT가 아닌 커맨드를 차단한다.
// RuleSet은 handshake에 1회만 호출되므로, ASSOCIATE를 통과시키면
// 이후 UDP 데이터그램은 검증 없이 임의 목적지로 릴레이된다.
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

// ipv4OnlyResolver는 hostname을 첫 IPv4 주소로 해석.
// IPv6-only 호스트는 에러 → lib이 RepHostUnreachable 응답.
type ipv4OnlyResolver struct{}

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

// slogLogger는 lib의 printf 스타일 Errorf를 slog로 어댑트.
type slogLogger struct {
	log *slog.Logger
}

func (l *slogLogger) Errorf(format string, args ...any) {
	l.log.Error(fmt.Sprintf(format, args...))
}

// NewSOCKS5Server: No-Auth만 허용 (Unix 소켓 권한이 실제 인증 경계).
// DialTimeout으로 black-hole 목적지에서 커널 기본(~75s) 대신 빠르게 실패시킴.
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
