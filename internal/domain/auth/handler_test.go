package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

func TestAllowedFrontendOrigin(t *testing.T) {
	tests := []struct {
		name        string
		frontendURL string
		pattern     string
		raw         string
		want        string
		ok          bool
	}{
		{
			name:        "configured frontend origin",
			frontendURL: "https://frontend.example.com",
			raw:         "https://frontend.example.com",
			want:        "https://frontend.example.com",
			ok:          true,
		},
		{
			name:        "configured localhost http origin",
			frontendURL: "http://localhost:4173",
			raw:         "http://localhost:4173",
			want:        "http://localhost:4173",
			ok:          true,
		},
		{
			name:    "localhost http origin from configured pattern",
			pattern: `^localhost:4173$`,
			raw:     "http://localhost:4173",
			want:    "http://localhost:4173",
			ok:      true,
		},
		{
			name:    "loopback ipv4 http origin from configured pattern",
			pattern: `^127[.]0[.]0[.]1:4173$`,
			raw:     "http://127.0.0.1:4173",
			want:    "http://127.0.0.1:4173",
			ok:      true,
		},
		{
			name:    "preview origin from configured pattern",
			pattern: `^preview-\d+\.frontend\.example\.com$`,
			raw:     "https://preview-21.frontend.example.com",
			want:    "https://preview-21.frontend.example.com",
			ok:      true,
		},
		{
			name:    "normalizes preview host case",
			pattern: `^preview-\d+\.frontend\.example\.com$`,
			raw:     "https://PREVIEW-21.FRONTEND.EXAMPLE.COM",
			want:    "https://preview-21.frontend.example.com",
			ok:      true,
		},
		{
			name:        "rejects http",
			frontendURL: "https://frontend.example.com",
			raw:         "http://frontend.example.com",
		},
		{
			name:    "rejects non-local http even when pattern matches",
			pattern: `^frontend[.]example[.]com$`,
			raw:     "http://frontend.example.com",
		},
		{
			name:        "rejects path",
			frontendURL: "https://frontend.example.com",
			raw:         "https://frontend.example.com/auth/callback",
		},
		{
			name:        "rejects query",
			frontendURL: "https://frontend.example.com",
			raw:         "https://frontend.example.com?next=https://evil.example",
		},
		{
			name:    "rejects lookalike preview host",
			pattern: `^preview-\d+\.frontend\.example\.com$`,
			raw:     "https://preview-21.frontend.example.com.evil.example",
		},
		{
			name: "rejects preview origin without configured pattern",
			raw:  "https://preview-21.frontend.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.frontendURL != "" {
				t.Setenv(envFrontendURL, tt.frontendURL)
			}
			if tt.pattern != "" {
				t.Setenv(envAllowedFrontendOriginPattern, tt.pattern)
			}

			got, ok := allowedFrontendOrigin(tt.raw)
			if ok != tt.ok {
				t.Fatalf("expected ok %v, got %v", tt.ok, ok)
			}
			if got != tt.want {
				t.Fatalf("expected origin %q, got %q", tt.want, got)
			}
		})
	}
}

func TestLoginIncludesAllowedRedirectOriginInState(t *testing.T) {
	t.Setenv(envAllowedFrontendOriginPattern, `^preview-\d+\.frontend\.example\.com$`)
	gin.SetMode(gin.TestMode)

	handler := NewHandler(NewService(&fakeRepo{users: map[string]*User{}}, &oauth2.Config{
		Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth"},
		ClientID: "client-id",
	}, NewTokenService("test-secret")))

	req := httptest.NewRequest(http.MethodGet, "/login?redirect_origin=https://preview-21.frontend.example.com", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Login(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}

	location := w.Header().Get("Location")
	if location == "" {
		t.Fatal("expected redirect location")
	}
	loginURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("failed to parse redirect location: %v", err)
	}
	rawState := loginURL.Query().Get("state")
	if rawState == "" {
		t.Fatal("expected oauth state")
	}
	stateBytes, err := base64.RawURLEncoding.DecodeString(rawState)
	if err != nil {
		t.Fatalf("failed to decode oauth state: %v", err)
	}
	var state oauthState
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatalf("failed to unmarshal oauth state: %v", err)
	}
	if state.RedirectOrigin != "https://preview-21.frontend.example.com" {
		t.Fatalf("expected redirect origin in state, got %q", state.RedirectOrigin)
	}
	if state.Nonce == "" {
		t.Fatal("expected nonce in state")
	}
}

func TestLoginRejectsInvalidRedirectOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(NewService(&fakeRepo{users: map[string]*User{}}, &oauth2.Config{}, NewTokenService("test-secret")))
	req := httptest.NewRequest(http.MethodGet, "/login?redirect_origin=https://evil.example", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Login(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if location := w.Header().Get("Location"); location != "" {
		t.Fatalf("expected no redirect location, got %q", location)
	}
}

func TestCallbackFailureRedirectsToStoredFrontendOrigin(t *testing.T) {
	t.Setenv(envAuthCookieSecure, "false")
	t.Setenv(envAllowedFrontendOriginPattern, `^preview-\d+\.frontend\.example\.com$`)
	gin.SetMode(gin.TestMode)

	stateBytes, err := json.Marshal(oauthState{
		Nonce:          "test-nonce",
		RedirectOrigin: "https://preview-21.frontend.example.com",
	})
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	rawState := base64.RawURLEncoding.EncodeToString(stateBytes)

	handler := NewHandler(NewService(&fakeRepo{users: map[string]*User{}}, &oauth2.Config{}, NewTokenService("test-secret")))
	req := httptest.NewRequest(http.MethodGet, "/callback?code=bad-code&state="+url.QueryEscape(rawState), nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Callback(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
	}
	wantLocation := "https://preview-21.frontend.example.com" + pathLoginError
	if got := w.Header().Get("Location"); got != wantLocation {
		t.Fatalf("expected redirect %q, got %q", wantLocation, got)
	}
}

func TestAuthCookieSameSite(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want http.SameSite
	}{
		{
			name: "defaults to lax",
			want: http.SameSiteLaxMode,
		},
		{
			name: "allows none",
			env:  "none",
			want: http.SameSiteNoneMode,
		},
		{
			name: "allows strict",
			env:  "strict",
			want: http.SameSiteStrictMode,
		},
		{
			name: "falls back to lax for unknown value",
			env:  "invalid",
			want: http.SameSiteLaxMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envAuthCookieSameSite, tt.env)

			if got := authCookieSameSite(); got != tt.want {
				t.Fatalf("expected SameSite %v, got %v", tt.want, got)
			}
		})
	}
}
