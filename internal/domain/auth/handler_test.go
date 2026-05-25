package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

func newAuthRouter(repo userRepository) (*gin.Engine, *TokenService) {
	tokenSvc := NewTokenService("test-secret")
	handler := NewHandler(NewService(repo, &oauth2.Config{}, tokenSvc))

	r := gin.New()
	rg := r.Group("/api/v1")
	handler.InitRoutes(rg)
	return r, tokenSvc
}

func findCookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestHandlerRefresh(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("rejects request without refresh cookie", func(t *testing.T) {
		router, _ := newAuthRouter(&fakeRepo{users: map[string]*User{}})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("rejects access token used as refresh cookie", func(t *testing.T) {
		repo := &fakeRepo{users: map[string]*User{
			"user@khu.ac.kr": {Email: "user@khu.ac.kr"},
		}}
		router, tokenSvc := newAuthRouter(repo)
		pair := issueAndStore(t, repo, tokenSvc, "user@khu.ac.kr")

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: cookieRefreshToken, Value: pair.AccessToken})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("rotates tokens and sets both cookies on success", func(t *testing.T) {
		repo := &fakeRepo{users: map[string]*User{
			"user@khu.ac.kr": {Email: "user@khu.ac.kr", Name: "User"},
		}}
		router, tokenSvc := newAuthRouter(repo)
		original := issueAndStore(t, repo, tokenSvc, "user@khu.ac.kr")

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: cookieRefreshToken, Value: original.RefreshToken})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}

		var body RefreshResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.AccessToken == "" {
			t.Fatal("expected non-empty access_token in body")
		}
		if body.ExpiresIn < 59*60 || body.ExpiresIn > 61*60 {
			t.Fatalf("expected expires_in ~3600, got %d", body.ExpiresIn)
		}

		accessCookie := findCookie(w.Result(), cookieAccessToken)
		if accessCookie == nil {
			t.Fatal("expected access_token cookie")
		}
		if accessCookie.Value != body.AccessToken {
			t.Fatal("access cookie should match body access_token")
		}
		if !accessCookie.HttpOnly || accessCookie.SameSite != http.SameSiteLaxMode {
			t.Error("access cookie missing HttpOnly or wrong SameSite")
		}

		refreshCookie := findCookie(w.Result(), cookieRefreshToken)
		if refreshCookie == nil {
			t.Fatal("expected refresh_token cookie to be rotated")
		}
		if refreshCookie.Value == original.RefreshToken {
			t.Fatal("expected new refresh token, got the original")
		}
		if !refreshCookie.HttpOnly || refreshCookie.SameSite != http.SameSiteLaxMode {
			t.Error("refresh cookie missing HttpOnly or wrong SameSite")
		}
		if refreshCookie.MaxAge < int(refreshTokenTTL.Seconds())-60 {
			t.Errorf("expected refresh Max-Age ~14d, got %d", refreshCookie.MaxAge)
		}
	})

	t.Run("rejects replay of old refresh token after rotation", func(t *testing.T) {
		repo := &fakeRepo{users: map[string]*User{
			"user@khu.ac.kr": {Email: "user@khu.ac.kr"},
		}}
		router, tokenSvc := newAuthRouter(repo)
		original := issueAndStore(t, repo, tokenSvc, "user@khu.ac.kr")

		// first refresh succeeds
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: cookieRefreshToken, Value: original.RefreshToken})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("first refresh expected 200, got %d", w.Code)
		}

		// replay original — must be rejected
		req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: cookieRefreshToken, Value: original.RefreshToken})
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 on replay, got %d", w.Code)
		}
	})

	t.Run("rejects when user no longer exists", func(t *testing.T) {
		router, tokenSvc := newAuthRouter(&fakeRepo{users: map[string]*User{}})
		pair, err := tokenSvc.GenerateAuthTokens("ghost@khu.ac.kr")
		if err != nil {
			t.Fatalf("GenerateAuthTokens: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: cookieRefreshToken, Value: pair.RefreshToken})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})
}

func TestHandlerLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("returns 204, clears server jti, expires both cookies", func(t *testing.T) {
		email := "user@khu.ac.kr"
		repo := &fakeRepo{users: map[string]*User{email: {Email: email}}}
		router, tokenSvc := newAuthRouter(repo)
		pair := issueAndStore(t, repo, tokenSvc, email)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: cookieRefreshToken, Value: pair.RefreshToken})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
		if repo.users[email].CurrentRefreshJTI != nil {
			t.Fatal("expected server-side jti to be cleared")
		}
		for _, name := range []string{cookieAccessToken, cookieRefreshToken} {
			c := findCookie(w.Result(), name)
			if c == nil {
				t.Fatalf("expected %s cookie in response", name)
			}
			if c.MaxAge >= 0 {
				t.Errorf("expected %s Max-Age < 0 (expired), got %d", name, c.MaxAge)
			}
			if !c.HttpOnly {
				t.Errorf("%s cookie must remain HttpOnly", name)
			}
		}
	})

	t.Run("rejects subsequent refresh after logout", func(t *testing.T) {
		email := "user@khu.ac.kr"
		repo := &fakeRepo{users: map[string]*User{email: {Email: email}}}
		router, tokenSvc := newAuthRouter(repo)
		pair := issueAndStore(t, repo, tokenSvc, email)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: cookieRefreshToken, Value: pair.RefreshToken})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("logout expected 204, got %d", w.Code)
		}

		req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: cookieRefreshToken, Value: pair.RefreshToken})
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for refresh after logout, got %d", w.Code)
		}
	})

	t.Run("succeeds even when no cookies are present", func(t *testing.T) {
		router, _ := newAuthRouter(&fakeRepo{users: map[string]*User{}})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
	})
}
