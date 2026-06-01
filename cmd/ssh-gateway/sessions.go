package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

var (
	ErrNonceUnknown   = errors.New("nonce unknown or already consumed")
	ErrNonceExpired   = errors.New("nonce expired")
	ErrCodeMismatch   = errors.New("code mismatch")
	ErrTooManyPending = errors.New("too many pending sessions")
)

const (
	maxCodeAttempts           = 5
	defaultMaxPendingSessions = 1024
)

// pendingSession is a single waiter created when a user opens an SSH session
// to the gateway. The keyboard-interactive callback parks on Wait until either
// the API webhook calls sessionStore.Resolve or the TTL elapses.
type pendingSession struct {
	Nonce        string
	Code         string
	codeAttempts int
	resolved     chan resolution
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
	mu         sync.Mutex
	m          map[string]*pendingSession
	ttl        time.Duration
	maxPending int
}

func newSessionStore(ttl time.Duration, maxPending int) *sessionStore {
	if maxPending <= 0 {
		maxPending = defaultMaxPendingSessions
	}
	return &sessionStore{m: make(map[string]*pendingSession), ttl: ttl, maxPending: maxPending}
}

// New creates a fresh nonce-backed pendingSession. The caller must call Wait
// from the keyboard-interactive callback. The TTL clock starts immediately.
func (s *sessionStore) New() (*pendingSession, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("rand: %w", err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(buf)
	code, err := randomCode()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if len(s.m) >= s.maxPending {
		s.mu.Unlock()
		return nil, ErrTooManyPending
	}
	p := &pendingSession{
		Nonce:    nonce,
		Code:     code,
		resolved: make(chan resolution, 1),
	}
	s.m[nonce] = p
	s.mu.Unlock()
	go s.expireAfter(p)
	return p, nil
}

func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("rand code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
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
func (s *sessionStore) Resolve(nonce, code, email string) error {
	s.mu.Lock()
	p, ok := s.m[nonce]
	if !ok {
		s.mu.Unlock()
		return ErrNonceUnknown
	}
	if p.Code != code {
		p.codeAttempts++
		if p.codeAttempts >= maxCodeAttempts {
			delete(s.m, nonce)
			close(p.resolved)
		}
		s.mu.Unlock()
		return ErrCodeMismatch
	}
	delete(s.m, nonce)
	s.mu.Unlock()
	select {
	case p.resolved <- resolution{email: email}:
	default:
	}
	return nil
}
