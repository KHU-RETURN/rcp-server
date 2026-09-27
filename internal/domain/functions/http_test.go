package functions

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

//go:embed testdata/http.wasm
var httpWasm []byte

func TestPublicHTTPAndKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := &memoryRepo{}
	svc, err := NewService(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	owner := uuid.New()
	fn, err := svc.Create(ctx, owner, "http", httpWasm)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	NewHandler(svc).InitPublicRoutes(r.Group("/api/v1"))
	url := "/api/v1/run/" + fn.ID.String() + "/hello?name=world"
	request := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if got := request(""); got.Code != http.StatusUnauthorized {
		t.Fatalf("missing key: %d", got.Code)
	}
	issued, err := svc.IssueKey(ctx, owner, fn.ID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IssueKey(ctx, uuid.New(), fn.ID, 7); err != ErrNotFound {
		t.Fatalf("other owner changed key: %v", err)
	}
	if _, err := svc.IssueKey(ctx, owner, fn.ID, 91); err != ErrInvalidExpiry {
		t.Fatalf("invalid expiry accepted: %v", err)
	}
	if !strings.HasPrefix(issued.Key, "rcpf_") || strings.Contains(string(repo.item.KeyHash), issued.Key) {
		t.Fatal("key was not generated or stored safely")
	}
	if got := request(issued.Key); got.Code != http.StatusCreated || got.Header().Get("X-Function") != "ok" || got.Body.String() != `{"ok":true}` {
		t.Fatalf("HTTP response: %d %s", got.Code, got.Body.String())
	}
	if got := request("wrong"); got.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d", got.Code)
	}
	rotated, err := svc.IssueKey(ctx, owner, fn.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Key == issued.Key {
		t.Fatal("rotation reused key")
	}
	if got := request(issued.Key); got.Code != http.StatusUnauthorized {
		t.Fatalf("old key still works: %d", got.Code)
	}
	if got := request(rotated.Key); got.Code != http.StatusCreated {
		t.Fatalf("rotated key failed: %d", got.Code)
	}
	now = now.Add(24 * time.Hour)
	if got := request(rotated.Key); got.Code != http.StatusUnauthorized {
		t.Fatalf("expired key works: %d", got.Code)
	}
	now = now.Add(-time.Hour)
	if err := svc.RevokeKey(ctx, owner, fn.ID); err != nil {
		t.Fatal(err)
	}
	if got := request(rotated.Key); got.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key works: %d", got.Code)
	}
	encoded, _ := json.Marshal(repo.item)
	if strings.Contains(string(encoded), "key_hash") || strings.Contains(string(encoded), issued.Key) {
		t.Fatal("key leaked in function response")
	}
}

func TestPublicHTTPResponseHeaderFiltering(t *testing.T) {
	for _, name := range []string{"Set-Cookie", "Content-Length", "Transfer-Encoding", "Connection", "X-Rcp-Secret"} {
		if safeResponseHeader(name, "value") {
			t.Errorf("unsafe response header allowed: %s", name)
		}
	}
	if safeResponseHeader("X-Good", "bad\r\nInjected: yes") {
		t.Fatal("header injection allowed")
	}
	if !sensitiveRequestHeader("Cookie") || !sensitiveRequestHeader("Authorization") {
		t.Fatal("platform secrets forwarded")
	}
}

func TestHTTPEventForwardsRequestWithoutPlatformCredentials(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/items?q=1", nil)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Cookie", "rcp_session=secret")
	event := newHTTPEvent(req, "/items", []byte{0xff, 0x00})
	if event.Method != http.MethodPost || event.Path != "/items" || event.Query != "q=1" || !event.IsBase64Encoded || event.Body != "/wA=" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.Headers["Content-Type"] != "application/octet-stream" || event.Headers["Authorization"] != "" || event.Headers["Cookie"] != "" {
		t.Fatalf("unsafe event headers: %+v", event.Headers)
	}
}
