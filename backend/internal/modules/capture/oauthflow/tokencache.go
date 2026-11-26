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
	now   func() time.Time
	limit int

	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	accessToken string
	replaceAt   time.Time
}

// NewTokenCache builds a cache that reads the time from now and holds at most
// limit tokens. Past it the token due soonest is dropped, which costs that
// credential one extra mint and nothing else.
func NewTokenCache(now func() time.Time, limit int) *TokenCache {
	return &TokenCache{now: now, limit: max(limit, 1), tokens: map[string]cachedToken{}}
}

// CacheKey derives a key from the parts that decide which token a grant
// returns. Hashed, so a client secret never sits in the map as a key.
func CacheKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// SharedTokenLimit bounds a cache of shared-mailbox app tokens.
const SharedTokenLimit = 128

// Token returns the cached token for key, or mints, keeps and returns a new
// one. A token with no stated lifetime is used once and not kept.
func (c *TokenCache) Token(ctx context.Context, key string, mint func(context.Context) (MintedToken, error)) (string, error) {
	if cached, ok := c.lookup(key); ok {
		return cached.AccessToken, nil
	}
	minted, err := mint(ctx)
	if err != nil {
		return "", err
	}
	c.keep(key, minted)
	return minted.AccessToken, nil
}

// lookup returns the token kept for key and the time left before it is due
// for replacement.
func (c *TokenCache) lookup(key string) (MintedToken, bool) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropExpired(now)
	cached, ok := c.tokens[key]
	if !ok {
		return MintedToken{}, false
	}
	return MintedToken{AccessToken: cached.accessToken, ExpiresIn: cached.replaceAt.Sub(now)}, true
}

// Drop forgets the token kept for key, for a token the provider stopped
// accepting before its stated lifetime ran out.
func (c *TokenCache) Drop(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, key)
}

// keep stores one minted token, evicting the one due soonest when the cache is
// full. A token with no stated lifetime is not kept.
func (c *TokenCache) keep(key string, minted MintedToken) {
	if minted.ExpiresIn <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	token := cachedToken{accessToken: minted.AccessToken, replaceAt: now.Add(replaceAfter(minted.ExpiresIn))}
	c.dropExpired(now)
	if _, replacing := c.tokens[key]; !replacing && len(c.tokens) >= c.limit {
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
