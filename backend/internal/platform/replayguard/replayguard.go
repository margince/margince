// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package replayguard remembers one-use ids, such as a signed token's id, in
// Redis, so every API replica refuses an id any of them has seen.
package replayguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

// keyPrefix namespaces the ids beside the bus's own keys.
const keyPrefix = "gw:once:"

// Guard holds each id until its time to live ends.
type Guard struct{ rdb *redis.Client }

// New builds a guard over rdb.
func New(rdb *redis.Client) Guard { return Guard{rdb: rdb} }

// FirstUse records key for ttl and reports whether it was unseen. The key is
// hashed, so a caller-chosen id sets neither the key's size nor its spelling.
// A ttl under a second is raised to one, because Redis keeps a key with no
// expiry forever.
func (g Guard) FirstUse(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return g.rdb.SetNX(ctx, stored(key), 1, max(ttl, time.Second)).Result()
}

// Release forgets key, so a use that failed before it took effect can be
// made again.
func (g Guard) Release(ctx context.Context, key string) error {
	return g.rdb.Del(ctx, stored(key)).Err()
}

func stored(key string) string {
	sum := sha256.Sum256([]byte(key))
	return keyPrefix + hex.EncodeToString(sum[:])
}
