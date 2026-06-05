package main

import (
	"context"
	"sync"
	"time"
)

type cachedFixedIPResolver struct {
	next  fixedIPResolver
	ttl   time.Duration
	now   func() time.Time
	mu    sync.RWMutex
	cache map[string]fixedIPCacheEntry
}

type fixedIPCacheEntry struct {
	ip        string
	expiresAt time.Time
}

func newCachedFixedIPResolver(next fixedIPResolver, ttl time.Duration) *cachedFixedIPResolver {
	return &cachedFixedIPResolver{
		next:  next,
		ttl:   ttl,
		now:   time.Now,
		cache: make(map[string]fixedIPCacheEntry),
	}
}

func (c *cachedFixedIPResolver) ResolveFixedIPv4(ctx context.Context, openstackID string) (string, error) {
	now := c.now()
	c.mu.RLock()
	entry, ok := c.cache[openstackID]
	c.mu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.ip, nil
	}

	ip, err := c.next.ResolveFixedIPv4(ctx, openstackID)
	if err != nil {
		if ok && entry.ip != "" {
			return entry.ip, nil
		}
		return "", err
	}

	c.mu.Lock()
	c.cache[openstackID] = fixedIPCacheEntry{ip: ip, expiresAt: now.Add(c.ttl)}
	c.mu.Unlock()
	return ip, nil
}
