package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSessionStore_NewNonce_Unique(t *testing.T) {
	s := newSessionStore(time.Minute, defaultMaxPendingSessions)
	a, _ := s.New()
	b, _ := s.New()
	if a.Nonce == b.Nonce {
		t.Fatal("two nonces collided")
	}
	if len(a.Nonce) < 32 {
		t.Fatalf("nonce too short: %q", a.Nonce)
	}
	if len(a.Code) != 6 {
		t.Fatalf("code should be six digits, got %q", a.Code)
	}
	for _, r := range a.Code {
		if r < '0' || r > '9' {
			t.Fatalf("code should be numeric, got %q", a.Code)
		}
	}
}

func TestSessionStore_ResolveDelivers(t *testing.T) {
	s := newSessionStore(time.Minute, defaultMaxPendingSessions)
	p, _ := s.New()
	go func() {
		time.Sleep(10 * time.Millisecond)
		if err := s.Resolve(p.Nonce, p.Code, "user@khu.ac.kr"); err != nil {
			t.Errorf("resolve: %v", err)
		}
	}()
	email, err := p.Wait(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if email != "user@khu.ac.kr" {
		t.Fatalf("got %q", email)
	}
}

func TestSessionStore_ResolveOnce(t *testing.T) {
	s := newSessionStore(time.Minute, defaultMaxPendingSessions)
	p, _ := s.New()
	if err := s.Resolve(p.Nonce, p.Code, "x@y"); err != nil {
		t.Fatal(err)
	}
	if err := s.Resolve(p.Nonce, p.Code, "x@y"); !errors.Is(err, ErrNonceUnknown) {
		t.Fatalf("second resolve should be ErrNonceUnknown, got %v", err)
	}
}

func TestSessionStore_RejectsWrongCode(t *testing.T) {
	s := newSessionStore(time.Minute, defaultMaxPendingSessions)
	p, _ := s.New()
	if err := s.Resolve(p.Nonce, "000000", "x@y"); !errors.Is(err, ErrCodeMismatch) {
		t.Fatalf("wrong code should be ErrCodeMismatch, got %v", err)
	}
	if err := s.Resolve(p.Nonce, p.Code, "x@y"); err != nil {
		t.Fatalf("correct code after mismatch: %v", err)
	}
}

func TestSessionStore_CodeMismatchLimitConsumesNonce(t *testing.T) {
	s := newSessionStore(time.Minute, defaultMaxPendingSessions)
	p, _ := s.New()
	wrong := "000000"
	if p.Code == wrong {
		wrong = "111111"
	}
	for i := 0; i < maxCodeAttempts; i++ {
		if err := s.Resolve(p.Nonce, wrong, "x@y"); !errors.Is(err, ErrCodeMismatch) {
			t.Fatalf("attempt %d got %v", i+1, err)
		}
	}
	if err := s.Resolve(p.Nonce, p.Code, "x@y"); !errors.Is(err, ErrNonceUnknown) {
		t.Fatalf("nonce should be consumed after code mismatch limit, got %v", err)
	}
}

func TestSessionStore_Expiry(t *testing.T) {
	s := newSessionStore(20*time.Millisecond, defaultMaxPendingSessions)
	p, _ := s.New()
	_, err := p.Wait(100 * time.Millisecond)
	if err == nil || !errors.Is(err, ErrNonceExpired) {
		t.Fatalf("expected ErrNonceExpired, got %v", err)
	}
	if err := s.Resolve(p.Nonce, p.Code, "x"); !errors.Is(err, ErrNonceUnknown) {
		t.Fatalf("post-expiry resolve: got %v", err)
	}
}

func TestSessionStore_NonceLooksRandom(t *testing.T) {
	s := newSessionStore(time.Minute, defaultMaxPendingSessions)
	p, _ := s.New()
	if strings.ContainsAny(p.Nonce, "+/=") {
		t.Errorf("nonce should be base64url (no +/=): %q", p.Nonce)
	}
}

func TestSessionStore_RejectsWhenPendingLimitReached(t *testing.T) {
	s := newSessionStore(time.Minute, 1)
	p, err := s.New()
	if err != nil {
		t.Fatalf("first new: %v", err)
	}
	if _, err := s.New(); !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("second new should be ErrTooManyPending, got %v", err)
	}
	if err := s.Resolve(p.Nonce, p.Code, "user@khu.ac.kr"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := s.New(); err != nil {
		t.Fatalf("new after resolve: %v", err)
	}
}
