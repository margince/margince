// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oauthflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// refreshHeadway is how long before expiry a cached token is replaced. A
// request that starts on it then never reaches the provider expired.
const refreshHeadway = 5 * time.Minute

// TokenCache keeps minted tokens until shortly before they expire. Minting
// runs outside the lock, so one slow token endpoint never holds up another
// credential's sync; two callers on one cold key may both mint.
type TokenCache struct {
	now func() time.Time

	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	accessToken string
	replaceAt   time.Time
}

// NewTokenCache builds a cache that reads the time from now.
func NewTokenCache(now func() time.Time) *TokenCache {
	return &TokenCache{now: now, tokens: map[string]cachedToken{}}
}

// CacheKey derives a key from the parts that decide which token a grant
// returns. Hashed, so a client secret never sits in the map as a key.
func CacheKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// maxCachedTokens bounds the cache. Past it the token due soonest is dropped,
// which costs that credential one extra mint and nothing else.
const maxCachedTokens = 128

// Token returns the cached token for key, or mints, keeps and returns a new
// one. A token with no stated lifetime is used once and not kept.
func (c *TokenCache) Token(ctx context.Context, key string, mint func(context.Context) (MintedToken, error)) (string, error) {
	now := c.now()
	c.mu.Lock()
	c.dropExpired(now)
	cached, ok := c.tokens[key]
	c.mu.Unlock()
	if ok {
		return cached.accessToken, nil
	}
	minted, err := mint(ctx)
	if err != nil {
		return "", err
	}
	if minted.ExpiresIn > 0 {
		c.keep(key, cachedToken{accessToken: minted.AccessToken, replaceAt: now.Add(replaceAfter(minted.ExpiresIn))})
	}
	return minted.AccessToken, nil
}

// keep stores one token, evicting the one due soonest when the cache is full.
func (c *TokenCache) keep(key string, token cachedToken) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropExpired(c.now())
	if _, replacing := c.tokens[key]; !replacing && len(c.tokens) >= maxCachedTokens {
		soonest := ""
		for k, t := range c.tokens {
			if soonest == "" || t.replaceAt.Before(c.tokens[soonest].replaceAt) {
				soonest = k
			}
		}
		delete(c.tokens, soonest)
	}
	c.tokens[key] = token
}

// dropExpired removes every token past its time. The caller holds mu.
func (c *TokenCache) dropExpired(now time.Time) {
	for k, t := range c.tokens {
		if !now.Before(t.replaceAt) {
			delete(c.tokens, k)
		}
	}
}

// replaceAfter is the headway before expiry, or half the lifetime of a token
// too short to spare it.
func replaceAfter(lifetime time.Duration) time.Duration {
	if lifetime <= refreshHeadway {
		return lifetime / 2
	}
	return lifetime - refreshHeadway
}
