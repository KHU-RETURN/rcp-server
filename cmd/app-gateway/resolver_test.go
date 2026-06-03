package main

import (
	"strings"
	"testing"
)

func TestFixedIPv4FromAddressesSelectsFixedIPv4(t *testing.T) {
	ip, err := fixedIPv4FromAddresses(map[string]any{
		"private": []any{
			map[string]any{"addr": "10.0.0.8", "version": float64(4), "OS-EXT-IPS:type": "fixed"},
			map[string]any{"addr": "203.0.113.8", "version": float64(4), "OS-EXT-IPS:type": "floating"},
		},
	}, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ip != "10.0.0.8" {
		t.Fatalf("ip got %q", ip)
	}
}

func TestFixedIPv4FromAddressesFiltersNetwork(t *testing.T) {
	ip, err := fixedIPv4FromAddresses(map[string]any{
		"other": []any{
			map[string]any{"addr": "10.1.0.8", "version": 4, "OS-EXT-IPS:type": "fixed"},
		},
		"tenant": []any{
			map[string]any{"addr": "10.0.0.8", "version": "4", "OS-EXT-IPS:type": "fixed"},
		},
	}, "tenant")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ip != "10.0.0.8" {
		t.Fatalf("ip got %q", ip)
	}
}

func TestFixedIPv4FromAddressesRejectsAmbiguousWithoutNetwork(t *testing.T) {
	_, err := fixedIPv4FromAddresses(map[string]any{
		"net-a": []any{
			map[string]any{"addr": "10.0.0.8", "version": 4, "OS-EXT-IPS:type": "fixed"},
		},
		"net-b": []any{
			map[string]any{"addr": "10.1.0.8", "version": 4, "OS-EXT-IPS:type": "fixed"},
		},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "multiple fixed IPv4") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestFixedIPv4FromAddressesRequiresAddress(t *testing.T) {
	_, err := fixedIPv4FromAddresses(map[string]any{
		"private": []any{
			map[string]any{"addr": "2001:db8::1", "version": 6, "OS-EXT-IPS:type": "fixed"},
		},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "no fixed IPv4") {
		t.Fatalf("expected missing fixed ip error, got %v", err)
	}
}
