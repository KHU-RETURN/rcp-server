package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNonceUnknown = errors.New("nonce unknown or already consumed")
	ErrNonceExpired = errors.New("nonce expired")
)

// pendingSession is a single waiter created when a user opens an SSH session
// to the gateway. The keyboard-interactive callback parks on Wait until either
// the API webhook calls sessionStore.Resolve or the TTL elapses.
type pendingSession struct {
	Nonce    string
	resolved chan resolution
}

type resolution struct {
	email string
	err   error
}

func (p *pendingSession) Wait(timeout time.Duration) (string, error) {
	select {
	case r, ok := <-p.resolved:
		if !ok {
			return "", ErrNonceExpired
		}
		return r.email, r.err
	case <-time.After(timeout):
		return "", ErrNonceExpired
	}
}

type sessionStore struct {
	mu  sync.Mutex
	m   map[string]*pendingSession
	ttl time.Duration
}

func newSessionStore(ttl time.Duration) *sessionStore {
	return &sessionStore{m: make(map[string]*pendingSession), ttl: ttl}
}

// New creates a fresh nonce-backed pendingSession. The caller must call Wait
// from the keyboard-interactive callback. The TTL clock starts immediately.
func (s *sessionStore) New() (*pendingSession, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("rand: %w", err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(buf)
	p := &pendingSession{
		Nonce:    nonce,
		resolved: make(chan resolution, 1),
	}
	s.mu.Lock()
	s.m[nonce] = p
	s.mu.Unlock()
	go s.expireAfter(p)
	return p, nil
}

func (s *sessionStore) expireAfter(p *pendingSession) {
	time.Sleep(s.ttl)
	s.mu.Lock()
	cur, ok := s.m[p.Nonce]
	if !ok || cur != p {
		s.mu.Unlock()
		return
	}
	delete(s.m, p.Nonce)
	s.mu.Unlock()
	close(p.resolved)
}

// Resolve delivers the authenticated email to the waiting session. One-time
// use: the entry is removed before delivering.
func (s *sessionStore) Resolve(nonce, email string) error {
	s.mu.Lock()
	p, ok := s.m[nonce]
	if !ok {
		s.mu.Unlock()
		return ErrNonceUnknown
	}
	delete(s.m, nonce)
	s.mu.Unlock()
	select {
	case p.resolved <- resolution{email: email}:
	default:
	}
	return nil
}
