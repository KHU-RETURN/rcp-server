package storage

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gophercloud/gophercloud"
)

func TestClientDownloadObjectPropagatesContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/test-container/index.html" {
			t.Fatalf("expected path /test-container/index.html, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8; swift=stored")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<!doctype html><html></html>"))
	}))
	defer server.Close()

	provider := &gophercloud.ProviderClient{
		TokenID:    "token",
		HTTPClient: *server.Client(),
	}
	provider.EndpointLocator = func(gophercloud.EndpointOpts) (string, error) {
		return server.URL + "/", nil
	}

	client := NewClient(provider)
	w := httptest.NewRecorder()

	if err := client.DownloadObject("test-container", "index.html", w); err != nil {
		t.Fatalf("DownloadObject: %v", err)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8; swift=stored" {
		t.Fatalf("expected response content type text/html; charset=utf-8; swift=stored, got %q", got)
	}
	if got := w.Body.String(); got != "<!doctype html><html></html>" {
		t.Fatalf("unexpected response body: %q", got)
	}
}
