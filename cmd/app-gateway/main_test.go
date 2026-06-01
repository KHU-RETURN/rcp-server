package main

import (
	"errors"
	"log/slog"
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

func TestNewLoggerAcceptsKnownLevels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "unknown"} {
		if got := newLogger(level); got == nil {
			t.Fatalf("newLogger(%q) returned nil", level)
		}
	}
}

func TestNewLoggerIsUsable(t *testing.T) {
	logger := newLogger("debug")
	if !logger.Enabled(nil, slog.LevelDebug) {
		t.Fatalf("debug logger should enable debug")
	}
}
