package auth

import (
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

func TestLoginStoresAllowedRedirectOrigin(t *testing.T) {
	t.Setenv(envAuthCookieSecure, "false")
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

	cookies := w.Result().Cookies()
	var got *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == cookieFrontendURL {
			got = cookie
			break
		}
	}
	if got == nil {
		t.Fatal("expected frontend URL cookie")
	}
	gotValue, err := url.QueryUnescape(got.Value)
	if err != nil {
		t.Fatalf("failed to unescape cookie value: %v", err)
	}
	if gotValue != "https://preview-21.frontend.example.com" {
		t.Fatalf("expected cookie value to be PR preview origin, got %q", gotValue)
	}
	if got.Path != cookiePathRoot {
		t.Fatalf("expected cookie path %q, got %q", cookiePathRoot, got.Path)
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

	handler := NewHandler(NewService(&fakeRepo{users: map[string]*User{}}, &oauth2.Config{}, NewTokenService("test-secret")))
	req := httptest.NewRequest(http.MethodGet, "/callback?code=bad-code", nil)
	req.AddCookie(&http.Cookie{
		Name:  cookieFrontendURL,
		Value: "https://preview-21.frontend.example.com",
	})
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
