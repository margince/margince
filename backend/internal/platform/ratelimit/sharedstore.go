// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// countScript opens the window on the event that fills it, so the span is
// measured from the first event exactly as the in-process store measures it.
// INCR and the expiry must be one round trip: an INCR whose PEXPIRE is lost
// between calls leaves a key that never expires, and that key holds its count
// forever — a caller limited once would be limited for the life of the server.
var countScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[1])) end
return n`)

// sharedTimeout bounds how long any one of the three entry points may wait on
// Redis. They take no context — Allow(key) is called from deep inside handlers
// that have one, and threading it through would be the interface change this
// is deliberately not — so the budget is stated here instead. It is short
// because the alternative to a fast answer is not a slow one: it is a request
// path that blocks on an unreachable counter, which is the outage the ceiling
// existed to prevent, arriving by a different road.
const sharedTimeout = 250 * time.Millisecond

// sharedStore counts in Redis, so every replica reads and writes ONE window
// per key and N replicas enforce one ceiling rather than N.
type sharedStore struct{ rdb *redis.Client }

func (s *sharedStore) count(key string, span time.Duration, _ time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sharedTimeout)
	defer cancel()
	n, err := countScript.Run(ctx, s.rdb, []string{key}, span.Milliseconds()).Int()
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *sharedStore) peek(key string, _ time.Duration, _ time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sharedTimeout)
	defer cancel()
	n, err := s.rdb.Get(ctx, key).Int()
	if errors.Is(err, redis.Nil) {
		// No window open. Not an outage: a key with no events is the ordinary
		// case on every first request, and reporting it as one would make the
		// security posture refuse everybody.
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

// forget deletes every key this Limiter owns.
//
// SCAN rather than a tracked key set: this runs on the non-production data
// reset, off any request path, and a set maintained on every count would cost
// a second write per event forever to serve a wipe. UNLINK rather than DEL so
// a large prefix is reclaimed off the server's request loop.
func (s *sharedStore) forget(prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), sharedTimeout)
	defer cancel()

	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, prefix+"*", scanBatch).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.rdb.Unlink(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

// scanBatch is a hint, not a page size: SCAN may return more or fewer. It is
// large enough that a wipe of a busy prefix is a few round trips rather than
// hundreds, and small enough not to block the server on one reply.
const scanBatch = 500
