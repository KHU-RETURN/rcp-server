package main

import (
	"strings"
	"testing"
	"time"
)

func TestFormatAuthPromptShowsBrowserAuthSteps(t *testing.T) {
	prompt := formatAuthPrompt("https://khu-return.com", "nonce-1", "363161", 5*time.Minute)

	want := []string{
		"RCP SSH browser authentication required.",
		"1. Open this URL in your browser:",
		"https://khu-return.com/ssh-auth?s=nonce-1",
		"2. Enter this 6-digit code on the auth page: 363161",
		"Waiting for browser authentication. Timeout: 5m0s",
	}
	for _, s := range want {
		if !strings.Contains(prompt, s) {
			t.Fatalf("prompt missing %q:\n%s", s, prompt)
		}
	}
}

func TestFormatAuthPromptDoesNotEmitTerminalEscapeSequences(t *testing.T) {
	prompt := formatAuthPrompt("https://khu-return.com", "nonce-1", "363161", 5*time.Minute)

	if strings.Contains(prompt, "\x1b") {
		t.Fatalf("prompt should not include ESC bytes:\n%q", prompt)
	}
	if strings.Contains(prompt, "\a") {
		t.Fatalf("prompt should not include BEL bytes:\n%q", prompt)
	}
}
