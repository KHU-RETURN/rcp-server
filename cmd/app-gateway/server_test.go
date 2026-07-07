package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeRepo struct {
	mapping *AppMapping
	err     error
	calls   int
}

func (f *fakeRepo) FindByHost(context.Context, string) (*AppMapping, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.mapping == nil {
		return nil, errNotFound
	}
	return f.mapping, nil
}

type fakeResolver struct {
	ip    string
	err   error
	calls int
}

func (f *fakeResolver) ResolveFixedIPv4(context.Context, string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.ip, nil
}

func testServer(t *testing.T, repo appRepo, resolver fixedIPResolver) *Server {
	t.Helper()
	srv, err := NewServer(
		&Config{NsProxySock: "/tmp/missing.sock"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		repo,
		resolver,
	)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	return srv
}

// countingTransport counts round trips so tests can prove one shared
// transport instance serves every request.
type countingTransport struct {
	calls int
}

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
	}, nil
}

func TestServerReturns404ForUnknownHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://missing.apps.khu-return.com/", nil)
	rr := httptest.NewRecorder()

	testServer(t, &fakeRepo{}, &fakeResolver{}).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status got %d", rr.Code)
	}
}

func TestServerReturns500ForRepoError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://abc.apps.khu-return.com/", nil)
	rr := httptest.NewRecorder()

	testServer(t, &fakeRepo{err: errors.New("db down")}, &fakeResolver{}).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status got %d", rr.Code)
	}
}

func TestServerReturns502ForResolverError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://abc.apps.khu-return.com/", nil)
	rr := httptest.NewRecorder()

	testServer(
		t,
		&fakeRepo{mapping: &AppMapping{Host: "abc.apps.khu-return.com", OpenstackID: "os-1"}},
		&fakeResolver{err: errors.New("no fixed ip")},
	).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status got %d", rr.Code)
	}
}

func TestServerReusesTransportAcrossRequests(t *testing.T) {
	repo := &fakeRepo{mapping: &AppMapping{Host: "abc.apps.khu-return.com", OpenstackID: "os-1"}}
	resolver := &fakeResolver{ip: "10.0.0.8"}
	srv := testServer(t, repo, resolver)

	first := srv.transport
	if first == nil {
		t.Fatal("transport not built in NewServer")
	}

	ct := &countingTransport{}
	srv.transport = ct

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://abc.apps.khu-return.com/", nil)
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d status got %d", i, rr.Code)
		}
	}
	if srv.transport != ct {
		t.Fatal("transport instance changed between requests")
	}
	if ct.calls != 2 {
		t.Fatalf("shared transport served %d requests, want 2", ct.calls)
	}
}

func TestServerCachesHostBackendResolution(t *testing.T) {
	repo := &fakeRepo{mapping: &AppMapping{Host: "abc.apps.khu-return.com", OpenstackID: "os-1"}}
	resolver := &fakeResolver{ip: "10.0.0.8"}
	srv := testServer(t, repo, resolver)
	srv.transport = &countingTransport{}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://abc.apps.khu-return.com/", nil)
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d status got %d", i, rr.Code)
		}
	}

	if repo.calls != 1 {
		t.Fatalf("repo hit %d times, want 1 (cached)", repo.calls)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver hit %d times, want 1 (cached)", resolver.calls)
	}
}

func TestNormalizeHost(t *testing.T) {
	if got := normalizeHost(" ABC.Apps.KHU-RETURN.com:18080 "); got != "abc.apps.khu-return.com" {
		t.Fatalf("got %q", got)
	}
}
