package http

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCloudflareTransportAddsAccessHeaders(t *testing.T) {
	t.Parallel()

	var got *http.Request
	transport := &cloudflareTransport{
		rt: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			got = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
		clientID:     "client-id",
		clientSecret: "client-secret",
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}

	if got == nil {
		t.Fatal("RoundTrip() did not forward the request")
	}

	if got.Header.Get("CF-Access-Client-Id") != "client-id" {
		t.Fatalf("unexpected CF-Access-Client-Id header: %q", got.Header.Get("CF-Access-Client-Id"))
	}

	if got.Header.Get("CF-Access-Client-Secret") != "client-secret" {
		t.Fatalf("unexpected CF-Access-Client-Secret header: %q", got.Header.Get("CF-Access-Client-Secret"))
	}

	if req.Header.Get("CF-Access-Client-Id") != "" {
		t.Fatalf("original request should not be mutated, got header %q", req.Header.Get("CF-Access-Client-Id"))
	}

	if req.Header.Get("CF-Access-Client-Secret") != "" {
		t.Fatalf("original request should not be mutated, got header %q", req.Header.Get("CF-Access-Client-Secret"))
	}
}

func TestNewCloudflareClientSetsDefaultTimeout(t *testing.T) {
	t.Parallel()

	client := NewCloudflareClient("client-id", "client-secret")
	if client.Timeout != defaultTimeout {
		t.Fatalf("unexpected timeout: got %s want %s", client.Timeout, defaultTimeout)
	}
}
