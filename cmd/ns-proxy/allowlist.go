package main

import (
	"fmt"
	"net"
	"strings"
)

// Allowlist는 목적지가 매치해야 하는 CIDR 집합.
type Allowlist struct {
	cidrs []*net.IPNet
}

// ParseCIDRs는 콤마 구분 CIDR 리스트를 파싱. 빈/공백 spec은 거부
// (fail closed: 빈 값을 "모두 허용"으로 해석하면 안 됨).
func ParseCIDRs(spec string) (*Allowlist, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, fmt.Errorf("CIDR spec is empty (fail closed)")
	}
	parts := strings.Split(spec, ",")
	cidrs := make([]*net.IPNet, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			return nil, fmt.Errorf("empty CIDR entry in spec %q", spec)
		}
		_, n, err := net.ParseCIDR(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", trimmed, err)
		}
		cidrs = append(cidrs, n)
	}
	return &Allowlist{cidrs: cidrs}, nil
}

// Contains는 ip가 허용 네트워크 중 하나에 속하면 true.
func (a *Allowlist) Contains(ip net.IP) bool {
	for _, n := range a.cidrs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
