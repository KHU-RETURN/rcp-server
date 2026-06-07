package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		header  string
		want    string
		wantErr bool
	}{
		{name: "valid bearer token", header: "Bearer token-123", want: "token-123"},
		{name: "case insensitive prefix", header: "bearer token-123", want: "token-123"},
		{name: "missing header", header: "", wantErr: true},
		{name: "missing token", header: "Bearer", wantErr: true},
		{name: "wrong scheme", header: "Basic token-123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractBearerToken(tt.header)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected token %q, got %q", tt.want, got)
			}
		})
	}
}

func TestAuthRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newProtectedRouter := func(repo userRepository) (*gin.Engine, string) {
		tokenSvc := NewTokenService("test-secret")
		handler := NewHandler(NewService(repo, &oauth2.Config{}, tokenSvc), nil, "")

		r := gin.New()
		protected := r.Group("/protected")
		protected.Use(handler.AuthRequired())
		protected.GET("", func(c *gin.Context) {
			email := c.GetString(ContextKeyUserEmail)
			c.JSON(http.StatusOK, gin.H{"email": email})
		})

		pair, err := tokenSvc.GenerateAuthTokens("user@khu.ac.kr")
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		return r, pair.AccessToken
	}

	t.Run("rejects request without authorization header", func(t *testing.T) {
		repo := &MockUserRepository{users: map[string]*User{}}
		router, _ := newProtectedRouter(repo)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("rejects invalid token", func(t *testing.T) {
		repo := &MockUserRepository{users: map[string]*User{}}
		router, _ := newProtectedRouter(repo)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("rejects refresh token", func(t *testing.T) {
		repo := &MockUserRepository{users: map[string]*User{
			"user@khu.ac.kr": {Email: "user@khu.ac.kr"},
		}}
		tokenSvc := NewTokenService("test-secret")
		handler := NewHandler(NewService(repo, &oauth2.Config{}, tokenSvc), nil, "")

		pair, err := tokenSvc.GenerateAuthTokens("user@khu.ac.kr")
		if err != nil {
			t.Fatalf("failed to generate refresh token: %v", err)
		}
		refreshToken := pair.RefreshToken

		r := gin.New()
		protected := r.Group("/protected")
		protected.Use(handler.AuthRequired())
		protected.GET("", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+refreshToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("rejects missing user", func(t *testing.T) {
		repo := &MockUserRepository{users: map[string]*User{}}
		router, token := newProtectedRouter(repo)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("allows valid access token", func(t *testing.T) {
		repo := &MockUserRepository{users: map[string]*User{
			"user@khu.ac.kr": {Email: "user@khu.ac.kr", Name: "User"},
		}}
		router, token := newProtectedRouter(repo)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["email"] != "user@khu.ac.kr" {
			t.Fatalf("expected email to be propagated, got %q", body["email"])
		}
	})

	t.Run("allows valid access token cookie", func(t *testing.T) {
		repo := &MockUserRepository{users: map[string]*User{
			"user@khu.ac.kr": {Email: "user@khu.ac.kr", Name: "User"},
		}}
		router, token := newProtectedRouter(repo)

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.AddCookie(&http.Cookie{Name: cookieAccessToken, Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
	})
}

func TestAdminRequiredUsesConfiguredAdminEmails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newAdminRouter := func(user *User) *gin.Engine {
		r := gin.New()
		protected := r.Group("/protected")
		protected.Use(func(c *gin.Context) {
			c.Set(ContextKeyUser, user)
		})
		protected.Use(AdminRequired())
		protected.GET("", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})
		return r
	}

	t.Run("allows configured admin email regardless of stored role", func(t *testing.T) {
		t.Setenv("RCP_ADMIN_EMAILS", "admin@return.dev, operator@return.dev")
		router := newAdminRouter(&User{Email: "operator@return.dev", Role: "user"})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("rejects unconfigured email even when stored role is admin", func(t *testing.T) {
		t.Setenv("RCP_ADMIN_EMAILS", "admin@return.dev")
		router := newAdminRouter(&User{Email: "student@return.dev", Role: "admin"})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", w.Code)
		}
	})
}
