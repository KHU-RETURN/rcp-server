package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRepo struct {
	mapping *AppMapping
	err     error
}

func (f fakeRepo) FindByHost(context.Context, string) (*AppMapping, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.mapping == nil {
		return nil, errNotFound
	}
	return f.mapping, nil
}

type fakeResolver struct {
	ip  string
	err error
}

func (f fakeResolver) ResolveFixedIPv4(context.Context, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.ip, nil
}

func testServer(repo appRepo, resolver fixedIPResolver) *Server {
	return NewServer(
		&Config{NsProxySock: "/tmp/missing.sock"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		repo,
		resolver,
	)
}

func TestServerReturns404ForUnknownHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://missing.apps.khu-return.com/", nil)
	rr := httptest.NewRecorder()

	testServer(fakeRepo{}, fakeResolver{}).ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status got %d", rr.Code)
	}
}

func TestServerReturns500ForRepoError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://abc.apps.khu-return.com/", nil)
	rr := httptest.NewRecorder()

	testServer(fakeRepo{err: errors.New("db down")}, fakeResolver{}).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status got %d", rr.Code)
	}
}

func TestServerReturns502ForResolverError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://abc.apps.khu-return.com/", nil)
	rr := httptest.NewRecorder()

	testServer(
		fakeRepo{mapping: &AppMapping{Host: "abc.apps.khu-return.com", OpenstackID: "os-1"}},
		fakeResolver{err: errors.New("no fixed ip")},
	).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status got %d", rr.Code)
	}
}

func TestNormalizeHost(t *testing.T) {
	if got := normalizeHost(" ABC.Apps.KHU-RETURN.com:18080 "); got != "abc.apps.khu-return.com" {
		t.Fatalf("got %q", got)
	}
}
