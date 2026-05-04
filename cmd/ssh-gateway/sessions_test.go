package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSessionStore_NewNonce_Unique(t *testing.T) {
	s := newSessionStore(time.Minute)
	a, _ := s.New()
	b, _ := s.New()
	if a.Nonce == b.Nonce {
		t.Fatal("two nonces collided")
	}
	if len(a.Nonce) < 32 {
		t.Fatalf("nonce too short: %q", a.Nonce)
	}
}

func TestSessionStore_ResolveDelivers(t *testing.T) {
	s := newSessionStore(time.Minute)
	p, _ := s.New()
	go func() {
		time.Sleep(10 * time.Millisecond)
		if err := s.Resolve(p.Nonce, "user@khu.ac.kr"); err != nil {
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
	s := newSessionStore(time.Minute)
	p, _ := s.New()
	if err := s.Resolve(p.Nonce, "x@y"); err != nil {
		t.Fatal(err)
	}
	if err := s.Resolve(p.Nonce, "x@y"); !errors.Is(err, ErrNonceUnknown) {
		t.Fatalf("second resolve should be ErrNonceUnknown, got %v", err)
	}
}

func TestSessionStore_Expiry(t *testing.T) {
	s := newSessionStore(20 * time.Millisecond)
	p, _ := s.New()
	_, err := p.Wait(100 * time.Millisecond)
	if err == nil || !errors.Is(err, ErrNonceExpired) {
		t.Fatalf("expected ErrNonceExpired, got %v", err)
	}
	if err := s.Resolve(p.Nonce, "x"); !errors.Is(err, ErrNonceUnknown) {
		t.Fatalf("post-expiry resolve: got %v", err)
	}
}

func TestSessionStore_NonceLooksRandom(t *testing.T) {
	s := newSessionStore(time.Minute)
	p, _ := s.New()
	if strings.ContainsAny(p.Nonce, "+/=") {
		t.Errorf("nonce should be base64url (no +/=): %q", p.Nonce)
	}
}
