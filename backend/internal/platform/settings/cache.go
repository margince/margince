// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
)

// CacheLifetime is how long a pooled read of a setting is remembered. Only a
// write in the same process clears the cache. The API and the worker are two
// processes, so a change reaches the other after at most this long. Five
// seconds is far below the shortest cadence and unnoticed on a toggle or format.
const CacheLifetime = 5 * time.Second

// valueCache remembers pooled setting reads by key. A key's row is shared by
// the whole installation, so the key alone identifies a value.
type valueCache struct {
	mu       sync.Mutex
	now      func() time.Time
	lifetime time.Duration
	rows     map[string]cachedRow
	// gen counts invalidations, so a read begun before one keeps nothing.
	gen map[string]uint64
	// pending counts open write transactions that changed a key. Nothing is
	// stored for it until each has ended.
	pending map[string]int
	// quiet is when a key may be stored again. A writer clears the cache before
	// it commits, and a read in between would keep the old row.
	quiet map[string]time.Time
}

type cachedRow struct {
	raw     json.RawMessage
	found   bool
	expires time.Time
}

func newValueCache(lifetime time.Duration, now func() time.Time) *valueCache {
	return &valueCache{
		now: now, lifetime: lifetime,
		rows: map[string]cachedRow{}, gen: map[string]uint64{}, pending: map[string]int{}, quiet: map[string]time.Time{},
	}
}

// read answers key from the cache while it is fresh, else from load.
func (c *valueCache) read(key string, load func() (json.RawMessage, bool, error)) (json.RawMessage, bool, error) {
	c.mu.Lock()
	if row, ok := c.rows[key]; ok && c.now().Before(row.expires) {
		c.mu.Unlock()
		return bytes.Clone(row.raw), row.found, nil
	}
	seen := c.gen[key]
	c.mu.Unlock()

	raw, found, err := load()
	if err != nil {
		return nil, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if at := c.now(); c.gen[key] == seen && c.pending[key] == 0 && !at.Before(c.quiet[key]) {
		c.rows[key] = cachedRow{raw: bytes.Clone(raw), found: found, expires: at.Add(c.lifetime)}
	}
	return raw, found, nil
}

func (c *valueCache) hold(keys []string) {
	c.forget(keys)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.pending[key]++
	}
}

func (c *valueCache) release(keys []string) {
	c.mu.Lock()
	for _, key := range keys {
		if c.pending[key] > 0 {
			c.pending[key]--
		}
	}
	c.mu.Unlock()
	c.forget(keys)
}

func (c *valueCache) forget(keys []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := c.now().Add(c.lifetime)
	for _, key := range keys {
		delete(c.rows, key)
		c.gen[key]++
		c.quiet[key] = until
	}
}

// caches holds one cache per pool that enabled it. A Store is a view that
// compose builds per consumer, so the cache cannot live on the Store.
var caches sync.Map

// EnableCache makes pooled reads through pool remember a setting for
// CacheLifetime. Off by default, so a test that writes a row with its own SQL
// reads it back at once.
func EnableCache(pool *pgxpool.Pool) { EnableCacheAt(pool, CacheLifetime, time.Now) }

// EnableCacheAt is EnableCache with a lifetime and clock a test controls.
func EnableCacheAt(pool *pgxpool.Pool, lifetime time.Duration, now func() time.Time) {
	caches.Store(pool, newValueCache(lifetime, now))
}

// cacheFor is the cache of pool, or nil when it has none.
func cacheFor(pool *pgxpool.Pool) *valueCache {
	held, ok := caches.Load(pool)
	if !ok {
		return nil
	}
	c, _ := held.(*valueCache)
	return c
}

// ForgetCached drops keys from every cache in the process. A caller that
// writes a row with its own SQL, outside a transaction, calls it.
func ForgetCached(keys ...string) {
	eachCache(func(c *valueCache) { c.forget(keys) })
}

// forgetCachedWith is the call every setting writer makes inside its
// transaction. Nothing is cached for keys until tx has ended, and the cache is
// cleared again then. Without a transaction end to hook, only the quiet
// period after the write protects the cache.
func forgetCachedWith(tx pgx.Tx, keys ...string) {
	var held []*valueCache
	eachCache(func(c *valueCache) {
		c.hold(keys)
		held = append(held, c)
	})
	if len(held) == 0 {
		return
	}
	release := func() {
		for _, c := range held {
			c.release(keys)
		}
	}
	if !database.AfterTx(tx, release) {
		release()
	}
}

func eachCache(fn func(*valueCache)) {
	caches.Range(func(_, held any) bool {
		if c, ok := held.(*valueCache); ok {
			fn(c)
		}
		return true
	})
}
