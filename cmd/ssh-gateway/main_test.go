package main

import (
	"errors"
	"os"
	"testing"
)

func TestLoadLocalEnvIgnoresMissingEnvFile(t *testing.T) {
	old := loadDotenv
	loadDotenv = func(filenames ...string) error {
		return os.ErrNotExist
	}
	defer func() { loadDotenv = old }()

	if err := loadLocalEnv(); err != nil {
		t.Fatalf("expected missing .env to be ignored, got %v", err)
	}
}

func TestLoadLocalEnvReturnsOtherErrors(t *testing.T) {
	want := errors.New("permission denied")
	old := loadDotenv
	loadDotenv = func(filenames ...string) error {
		return want
	}
	defer func() { loadDotenv = old }()

	if err := loadLocalEnv(); !errors.Is(err, want) {
		t.Fatalf("got %v want %v", err, want)
	}
}

func TestFixedIPv4FromAddressesIgnoresFloatingIPs(t *testing.T) {
	got, err := fixedIPv4FromAddresses(map[string]any{
		"public": []any{
			map[string]any{
				"version":         float64(4),
				"addr":            "203.0.113.10",
				"OS-EXT-IPS:type": "floating",
			},
		},
		"tenant": []any{
			map[string]any{
				"version":         float64(4),
				"addr":            "10.0.0.7",
				"OS-EXT-IPS:type": "fixed",
			},
		},
	}, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "10.0.0.7" {
		t.Fatalf("got %q", got)
	}
}

func TestFixedIPv4FromAddressesRequiresFixedIP(t *testing.T) {
	_, err := fixedIPv4FromAddresses(map[string]any{
		"public": []any{
			map[string]any{
				"version":         float64(4),
				"addr":            "203.0.113.10",
				"OS-EXT-IPS:type": "floating",
			},
		},
	}, "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFixedIPv4FromAddressesRequiresNetworkWhenMultipleFixedIPs(t *testing.T) {
	_, err := fixedIPv4FromAddresses(map[string]any{
		"tenant-a": []any{
			map[string]any{
				"version":         float64(4),
				"addr":            "10.0.0.7",
				"OS-EXT-IPS:type": "fixed",
			},
		},
		"tenant-b": []any{
			map[string]any{
				"version":         float64(4),
				"addr":            "10.1.0.7",
				"OS-EXT-IPS:type": "fixed",
			},
		},
	}, "")
	if err == nil {
		t.Fatal("expected ambiguous fixed IPv4 error")
	}
}

func TestFixedIPv4FromAddressesSelectsConfiguredNetwork(t *testing.T) {
	got, err := fixedIPv4FromAddresses(map[string]any{
		"tenant-a": []any{
			map[string]any{
				"version":         float64(4),
				"addr":            "10.0.0.7",
				"OS-EXT-IPS:type": "fixed",
			},
		},
		"tenant-b": []any{
			map[string]any{
				"version":         float64(4),
				"addr":            "10.1.0.7",
				"OS-EXT-IPS:type": "fixed",
			},
		},
	}, "tenant-b")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "10.1.0.7" {
		t.Fatalf("got %q", got)
	}
}
