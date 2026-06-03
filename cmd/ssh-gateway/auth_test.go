package main

import (
	"encoding/base64"
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
		"2. Enter this 6-digit code on the auth page:",
		"\r\n363161\r\n",
		"Waiting for browser authentication. Timeout: 5m0s",
	}
	for _, s := range want {
		if !strings.Contains(prompt, s) {
			t.Fatalf("prompt missing %q:\n%s", s, prompt)
		}
	}
}

func TestFormatAuthPromptAddsTerminalConveniences(t *testing.T) {
	prompt := formatAuthPrompt("https://khu-return.com", "nonce-1", "363161", 5*time.Minute)
	url := "https://khu-return.com/ssh-auth?s=nonce-1"

	if !strings.Contains(prompt, "\x1b]8;;"+url+"\a"+url+"\x1b]8;;\a") {
		t.Fatalf("prompt should include OSC 8 hyperlink for URL:\n%q", prompt)
	}

	encodedCode := base64.StdEncoding.EncodeToString([]byte("363161"))
	if !strings.Contains(prompt, "\x1b]52;c;"+encodedCode+"\a") {
		t.Fatalf("prompt should include OSC 52 clipboard copy for code:\n%q", prompt)
	}
}
