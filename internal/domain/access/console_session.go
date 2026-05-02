package access

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

type consoleSession struct {
	Token         string
	OwnerID       uuid.UUID
	InstanceID    string
	Host          string
	Username      string
	Signer        ssh.Signer
	AuthorizedKey string
	ExpiresAt     time.Time
}

type consoleSessionStore struct {
	mu             sync.Mutex
	ttl            time.Duration
	sessions       map[string]consoleSession
	authorizedKeys map[authorizedKeyScope]map[string]time.Time
}

func newConsoleSessionStore(ttl time.Duration) *consoleSessionStore {
	return &consoleSessionStore{
		ttl:            ttl,
		sessions:       make(map[string]consoleSession),
		authorizedKeys: make(map[authorizedKeyScope]map[string]time.Time),
	}
}

func (s *consoleSessionStore) Create(session consoleSession) (*consoleSession, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	session.Token = token
	session.ExpiresAt = time.Now().Add(s.ttl)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteExpiredLocked(time.Now())
	s.sessions[token] = session
	s.addAuthorizedKeyLocked(session)
	return &session, nil
}

func (s *consoleSessionStore) Take(token string) (*consoleSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.deleteExpiredLocked(now)

	session, ok := s.sessions[token]
	if !ok {
		return nil, false
	}
	delete(s.sessions, token)

	if now.After(session.ExpiresAt) {
		return nil, false
	}
	return &session, true
}

func (s *consoleSessionStore) AuthorizedKeys(instanceID, username string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.deleteExpiredLocked(time.Now())
	scope := authorizedKeyScope{InstanceID: instanceID, Username: username}
	keys := s.authorizedKeys[scope]
	if len(keys) == 0 {
		return ""
	}

	result := ""
	for key := range keys {
		result += key + "\n"
	}
	return result
}

func (s *consoleSessionStore) DeleteAuthorizedKey(instanceID, username, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	scope := authorizedKeyScope{InstanceID: instanceID, Username: username}
	keys := s.authorizedKeys[scope]
	if len(keys) == 0 {
		return
	}
	delete(keys, key)
	if len(keys) == 0 {
		delete(s.authorizedKeys, scope)
	}
}

func (s *consoleSessionStore) deleteExpiredLocked(now time.Time) {
	for token, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, token)
		}
	}
	for scope, keys := range s.authorizedKeys {
		for key, expiresAt := range keys {
			if now.After(expiresAt) {
				delete(keys, key)
			}
		}
		if len(keys) == 0 {
			delete(s.authorizedKeys, scope)
		}
	}
}

func (s *consoleSessionStore) addAuthorizedKeyLocked(session consoleSession) {
	scope := authorizedKeyScope{InstanceID: session.InstanceID, Username: session.Username}
	if s.authorizedKeys[scope] == nil {
		s.authorizedKeys[scope] = make(map[string]time.Time)
	}
	s.authorizedKeys[scope][session.AuthorizedKey] = session.ExpiresAt
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

type authorizedKeyScope struct {
	InstanceID string
	Username   string
}
