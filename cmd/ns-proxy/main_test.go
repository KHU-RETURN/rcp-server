package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// mustAllowlist is a test helper that parses a CIDR spec and fatals on error.
// Tests across this package (server_test.go, main_test.go) can call it freely
// since all files share the same package main test binary.
func mustAllowlist(t *testing.T, cidr string) *Allowlist {
	t.Helper()
	al, err := ParseCIDRs(cidr)
	if err != nil {
		t.Fatalf("mustAllowlist(%q): %v", cidr, err)
	}
	return al
}

func TestNewLogger_LevelMapping(t *testing.T) {
	cases := []struct {
		in           string
		wantEnabled  slog.Level
		wantDisabled slog.Level // a level strictly below wantEnabled; zero value unused for "debug"
		checkBelow   bool       // whether to assert wantDisabled is NOT enabled
	}{
		{"debug", slog.LevelDebug, 0, false},             // nothing strictly below debug
		{"info", slog.LevelInfo, slog.LevelDebug, true},  // debug must be off
		{"warn", slog.LevelWarn, slog.LevelInfo, true},   // info must be off
		{"error", slog.LevelError, slog.LevelWarn, true}, // warn must be off
		{"", slog.LevelInfo, slog.LevelDebug, true},      // default → info; debug must be off
		{"weird", slog.LevelInfo, slog.LevelDebug, true}, // unknown → info; debug must be off
	}
	for _, c := range cases {
		log := newLogger(c.in)
		if !log.Enabled(context.Background(), c.wantEnabled) {
			t.Errorf("newLogger(%q): want %v enabled", c.in, c.wantEnabled)
		}
		if c.checkBelow && log.Enabled(context.Background(), c.wantDisabled) {
			t.Errorf("newLogger(%q): %v should NOT be enabled", c.in, c.wantDisabled)
		}
	}
}

func TestStatsLoop_EmitsAtStartAndInterval(t *testing.T) {
	cfg := &Config{
		AllowList:   mustAllowlist(t, "127.0.0.0/8"),
		MaxConns:    4,
		DialTimeout: time.Second,
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	srv := NewServer(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); statsLoop(ctx, srv, log, 50*time.Millisecond) }()

	// Wait long enough for at least 2 emissions (startup + 1 tick).
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-done

	// Count "stats" entries.
	n := strings.Count(buf.String(), `msg=stats`)
	if n < 2 {
		t.Errorf("got %d stats lines, want >=2", n)
	}
}
