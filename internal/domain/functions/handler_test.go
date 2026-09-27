package functions

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestFunctionDataHTTPAuthorization(t *testing.T) {
	ctx := context.Background()
	store, err := OpenDataStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(ctx, &memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	svc.SetDataStore(store)
	t.Cleanup(func() { _ = svc.Close(ctx) })
	owner := uuid.New()
	fn, err := svc.Create(ctx, owner, "data", echoWasm)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		user := owner
		if c.GetHeader("X-Test-Other-User") != "" {
			user = uuid.New()
		}
		c.Set(auth.ContextKeyUser, &auth.User{ID: user})
		c.Next()
	})
	NewHandler(svc).InitRoutes(r.Group("/api/v1"))
	url := "/api/v1/functions/" + fn.ID.String() + "/data/notes/first"
	req := httptest.NewRequest(http.MethodPut, url, bytes.NewBufferString(`{"text":"hello"}`))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("put: %d %s", resp.Code, resp.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Test-Other-User", "1")
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("cross-owner read: %d", resp.Code)
	}
	req = httptest.NewRequest(http.MethodGet, url, nil)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || !bytes.Contains(resp.Body.Bytes(), []byte(`"text":"hello"`)) {
		t.Fatalf("get: %d %s", resp.Code, resp.Body.String())
	}
}

func TestFunctionHTTPFlow(t *testing.T) {
	ctx := context.Background()
	svc, err := NewService(ctx, &memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	owner := uuid.New()
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(auth.ContextKeyUser, &auth.User{ID: owner}); c.Next() })
	NewHandler(svc).InitRoutes(r.Group("/api/v1"))

	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	if err := writer.WriteField("name", "echo"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "echo.wasm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(echoWasm); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/functions", &upload)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("deploy: %d %s", response.Code, response.Body.String())
	}
	var fn Function
	if err := json.Unmarshal(response.Body.Bytes(), &fn); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/functions/"+fn.ID.String()+"/invoke", bytes.NewBufferString(`{"message":"hello"}`))
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("invoke: %d %s", response.Code, response.Body.String())
	}
	var result Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "{\"MESSAGE\":\"HELLO\"}" {
		t.Fatalf("unexpected output: %+v", result)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/functions", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), echoWasm[:16]) {
		t.Fatalf("list should succeed without binary: %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/functions/"+fn.ID.String()+"/key", bytes.NewBufferString(`{"expires_in_days":7}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("issue key: %d %s", response.Code, response.Body.String())
	}
	var issued IssuedKey
	if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil || issued.Key == "" {
		t.Fatalf("key response: %v", err)
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/functions/"+fn.ID.String()+"/key", nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("revoke key: %d", response.Code)
	}
}

func TestSourceUploadHTTP(t *testing.T) {
	ctx := context.Background()
	svc, err := NewService(ctx, &memoryRepo{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(ctx) })
	svc.SetBuilder(fakeBuilder{wasm: echoWasm})
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(auth.ContextKeyUser, &auth.User{ID: uuid.New()}); c.Next() })
	NewHandler(svc).InitRoutes(r.Group("/api/v1"))
	var upload bytes.Buffer
	w := multipart.NewWriter(&upload)
	_ = w.WriteField("name", "echo")
	_ = w.WriteField("language", "rust")
	part, err := w.CreateFormFile("file", "echo.rs")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("fn main() {}"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/functions", &upload)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("deploy: %d %s", resp.Code, resp.Body.String())
	}
	var fn Function
	if err := json.Unmarshal(resp.Body.Bytes(), &fn); err != nil {
		t.Fatal(err)
	}
	if fn.Language != "rust" || len(fn.Source) != 0 {
		t.Fatalf("bad public response: %+v", fn)
	}
}
