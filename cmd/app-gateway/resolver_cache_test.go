package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type countingResolver struct {
	ip    string
	err   error
	calls int
}

func (c *countingResolver) ResolveFixedIPv4(context.Context, string) (string, error) {
	c.calls++
	if c.err != nil {
		return "", c.err
	}
	return c.ip, nil
}

func TestCachedFixedIPResolverCachesSuccessfulLookup(t *testing.T) {
	next := &countingResolver{ip: "10.0.0.8"}
	resolver := newCachedFixedIPResolver(next, time.Minute)

	for range 2 {
		got, err := resolver.ResolveFixedIPv4(context.Background(), "os-1")
		if err != nil {
			t.Fatalf("ResolveFixedIPv4 returned error: %v", err)
		}
		if got != "10.0.0.8" {
			t.Fatalf("ip got %q", got)
		}
	}

	if next.calls != 1 {
		t.Fatalf("underlying resolver calls got %d", next.calls)
	}
}

func TestCachedFixedIPResolverDoesNotCacheErrors(t *testing.T) {
	next := &countingResolver{err: errors.New("nova timeout")}
	resolver := newCachedFixedIPResolver(next, time.Minute)

	for range 2 {
		_, err := resolver.ResolveFixedIPv4(context.Background(), "os-1")
		if err == nil {
			t.Fatalf("expected error")
		}
	}

	if next.calls != 2 {
		t.Fatalf("underlying resolver calls got %d", next.calls)
	}
}

func TestCachedFixedIPResolverRefreshesAfterTTL(t *testing.T) {
	next := &countingResolver{ip: "10.0.0.8"}
	resolver := newCachedFixedIPResolver(next, time.Minute)
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	resolver.now = func() time.Time { return now }

	if _, err := resolver.ResolveFixedIPv4(context.Background(), "os-1"); err != nil {
		t.Fatalf("ResolveFixedIPv4 returned error: %v", err)
	}

	next.ip = "10.0.0.7"
	now = now.Add(time.Minute + time.Second)
	got, err := resolver.ResolveFixedIPv4(context.Background(), "os-1")
	if err != nil {
		t.Fatalf("ResolveFixedIPv4 returned error: %v", err)
	}
	if got != "10.0.0.7" {
		t.Fatalf("ip got %q", got)
	}
	if next.calls != 2 {
		t.Fatalf("underlying resolver calls got %d", next.calls)
	}
}

func TestCachedFixedIPResolverUsesStaleIPOnRefreshError(t *testing.T) {
	next := &countingResolver{ip: "10.0.0.8"}
	resolver := newCachedFixedIPResolver(next, time.Minute)
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	resolver.now = func() time.Time { return now }

	if _, err := resolver.ResolveFixedIPv4(context.Background(), "os-1"); err != nil {
		t.Fatalf("ResolveFixedIPv4 returned error: %v", err)
	}

	next.err = errors.New("nova timeout")
	now = now.Add(time.Minute + time.Second)
	got, err := resolver.ResolveFixedIPv4(context.Background(), "os-1")
	if err != nil {
		t.Fatalf("ResolveFixedIPv4 returned error: %v", err)
	}
	if got != "10.0.0.8" {
		t.Fatalf("ip got %q", got)
	}
	if next.calls != 2 {
		t.Fatalf("underlying resolver calls got %d", next.calls)
	}
}
