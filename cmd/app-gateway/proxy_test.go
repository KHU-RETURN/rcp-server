package main

import (
	"net/http"
	"net/http/httputil"
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

	out := req.Clone(req.Context())
	rp.Rewrite(&httputil.ProxyRequest{In: req, Out: out})

	if out.Host != "return.apps.khu-return.com" {
		t.Fatalf("Host got %q", out.Host)
	}
	if out.URL.Scheme != "http" {
		t.Fatalf("scheme got %q", out.URL.Scheme)
	}
	if out.URL.Host != "10.0.0.8:80" {
		t.Fatalf("target host got %q", out.URL.Host)
	}
	if out.URL.Path != "/path" || out.URL.RawQuery != "q=1" {
		t.Fatalf("path/query got %q?%q", out.URL.Path, out.URL.RawQuery)
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
