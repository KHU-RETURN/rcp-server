package storage

import (
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gophercloud/gophercloud"
)

func TestClientDownloadObjectPropagatesResponseHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/test-container/index.html" {
			t.Fatalf("expected path /test-container/index.html, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", "28")
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Mon, 01 Jun 2026 00:00:00 GMT")
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
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("expected response content type text/html; charset=utf-8, got %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "28" {
		t.Fatalf("expected response content length 28, got %q", got)
	}
	if got := w.Header().Get("ETag"); got != `"abc123"` {
		t.Fatalf("expected response etag, got %q", got)
	}
	if got := w.Header().Get("Last-Modified"); got != "Mon, 01 Jun 2026 00:00:00 GMT" {
		t.Fatalf("expected response last modified, got %q", got)
	}
	if got := w.Body.String(); got != "<!doctype html><html></html>" {
		t.Fatalf("unexpected response body: %q", got)
	}
}

func TestClientDownloadObjectForcesAttachmentForActiveContent(t *testing.T) {
	tests := []struct {
		name               string
		objectName         string
		contentType        string
		contentDisposition string
		wantFilename       string
	}{
		{
			name:         "html content type",
			objectName:   "index.html",
			contentType:  "text/html; charset=utf-8",
			wantFilename: "index.html",
		},
		{
			name:         "html extension",
			objectName:   "page.htm",
			contentType:  "application/octet-stream",
			wantFilename: "page.htm",
		},
		{
			name:               "svg overrides inline disposition",
			objectName:         "icon.svg",
			contentType:        "image/svg+xml",
			contentDisposition: `inline; filename="icon.svg"`,
			wantFilename:       "icon.svg",
		},
		{
			name:         "xhtml content type",
			objectName:   "document.bin",
			contentType:  "application/xhtml+xml",
			wantFilename: "document.bin",
		},
		{
			name:         "malformed html content type",
			objectName:   "legacy.txt",
			contentType:  "text/html; charset=utf-8\u201d",
			wantFilename: "legacy.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				if tt.contentDisposition != "" {
					w.Header().Set("Content-Disposition", tt.contentDisposition)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("body"))
			}))
			defer server.Close()

			client := newTestObjectStorageClient(server)
			w := httptest.NewRecorder()

			if err := client.DownloadObject("test-container", tt.objectName, w); err != nil {
				t.Fatalf("DownloadObject: %v", err)
			}

			disposition, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
			if err != nil {
				t.Fatalf("ParseMediaType: %v", err)
			}
			if disposition != "attachment" {
				t.Fatalf("expected attachment disposition, got %q", disposition)
			}
			if params["filename"] != tt.wantFilename {
				t.Fatalf("expected filename %q, got %q", tt.wantFilename, params["filename"])
			}
		})
	}
}

func TestClientDownloadObjectPreservesPassiveContentDisposition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", `inline; filename="photo.png"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("png"))
	}))
	defer server.Close()

	client := newTestObjectStorageClient(server)
	w := httptest.NewRecorder()

	if err := client.DownloadObject("test-container", "photo.png", w); err != nil {
		t.Fatalf("DownloadObject: %v", err)
	}
	if got := w.Header().Get("Content-Disposition"); got != `inline; filename="photo.png"` {
		t.Fatalf("expected passive content disposition to be preserved, got %q", got)
	}
}

func newTestObjectStorageClient(server *httptest.Server) *Client {
	provider := &gophercloud.ProviderClient{
		TokenID:    "token",
		HTTPClient: *server.Client(),
	}
	provider.EndpointLocator = func(gophercloud.EndpointOpts) (string, error) {
		return server.URL + "/", nil
	}
	return NewClient(provider)
}
