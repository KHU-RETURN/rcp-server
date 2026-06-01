package main

import (
	"net/http"
	"testing"
)

func TestNewReverseProxyPreservesOriginalHost(t *testing.T) {
	rp, err := newReverseProxy("10.0.0.8", 80, "/tmp/missing.sock")
	if err != nil {
		t.Fatalf("newReverseProxy returned error: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "http://return.apps.khu-return.com/path?q=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "return.apps.khu-return.com"

	rp.Director(req)

	if req.Host != "return.apps.khu-return.com" {
		t.Fatalf("Host got %q", req.Host)
	}
	if req.URL.Scheme != "http" {
		t.Fatalf("scheme got %q", req.URL.Scheme)
	}
	if req.URL.Host != "10.0.0.8:80" {
		t.Fatalf("target host got %q", req.URL.Host)
	}
	if req.URL.Path != "/path" || req.URL.RawQuery != "q=1" {
		t.Fatalf("path/query got %q?%q", req.URL.Path, req.URL.RawQuery)
	}
}

func TestTransportViaNSProxySetsDialer(t *testing.T) {
	tr, err := transportViaNSProxy("/tmp/missing.sock")
	if err != nil {
		t.Fatalf("transportViaNSProxy returned error: %v", err)
	}
	if tr.DialContext == nil {
		t.Fatalf("DialContext is nil")
	}
	if tr.ForceAttemptHTTP2 {
		t.Fatalf("ForceAttemptHTTP2 should be disabled for SOCKS-over-unix transport")
	}
}
