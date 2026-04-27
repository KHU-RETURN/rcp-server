package main

import (
	"fmt"
	"net"
	"strings"
)

// Allowlist holds a set of CIDR networks that destinations must match.
type Allowlist struct {
	cidrs []*net.IPNet
}

// ParseCIDRs parses a comma-separated CIDR list. An empty or whitespace-only
// spec is rejected (fail closed: empty must not silently allow everything).
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

// Contains returns true when ip falls inside any of the allowed networks.
func (a *Allowlist) Contains(ip net.IP) bool {
	for _, n := range a.cidrs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
