package main

import (
	"net"
	"testing"
)

func TestParseCIDRs_RejectsEmpty(t *testing.T) {
	if _, err := ParseCIDRs(""); err == nil {
		t.Fatal("expected error for empty spec, got nil")
	}
	if _, err := ParseCIDRs("   "); err == nil {
		t.Fatal("expected error for whitespace spec, got nil")
	}
}

func TestParseCIDRs_RejectsInvalid(t *testing.T) {
	if _, err := ParseCIDRs("not-a-cidr"); err == nil {
		t.Fatal("expected error for garbage, got nil")
	}
	if _, err := ParseCIDRs("192.168.0.0/16,bad/24"); err == nil {
		t.Fatal("expected error when one entry invalid, got nil")
	}
}

func TestParseCIDRs_RejectsEmptyEntries(t *testing.T) {
	cases := []string{
		"192.168.0.0/16,",
		",10.0.0.0/8",
		"192.168.0.0/16,,10.0.0.0/8",
		"  ,  ,  ",
	}
	for _, spec := range cases {
		if _, err := ParseCIDRs(spec); err == nil {
			t.Errorf("ParseCIDRs(%q) should error, got nil", spec)
		}
	}
}

func TestAllowlist_ContainsSingleCIDR(t *testing.T) {
	a, err := ParseCIDRs("192.168.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ip1 := net.ParseIP("192.168.1.10")
	if ip1 == nil {
		t.Fatal("test bug: net.ParseIP(\"192.168.1.10\") returned nil")
	}
	if !a.Contains(ip1) {
		t.Error("expected 192.168.1.10 to match 192.168.0.0/16")
	}
	ip2 := net.ParseIP("10.0.0.1")
	if ip2 == nil {
		t.Fatal("test bug: net.ParseIP(\"10.0.0.1\") returned nil")
	}
	if a.Contains(ip2) {
		t.Error("did not expect 10.0.0.1 to match 192.168.0.0/16")
	}
}

func TestAllowlist_ContainsMultipleCIDRs(t *testing.T) {
	a, err := ParseCIDRs("192.168.0.0/16, 10.42.0.0/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cases := []struct {
		ip      string
		allowed bool
	}{
		{"192.168.5.5", true},
		{"10.42.0.99", true},
		{"10.43.0.1", false},
		{"172.16.0.1", false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("test bug: net.ParseIP(%q) returned nil", c.ip)
		}
		got := a.Contains(ip)
		if got != c.allowed {
			t.Errorf("Contains(%s) = %v, want %v", c.ip, got, c.allowed)
		}
	}
}
