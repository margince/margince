// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package lastseen keeps when something last happened, in Redis, for as long
// as the moment can still change a decision. Every replica reads the same
// moment, and no database row is written for it.
package lastseen

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// keyPrefix namespaces the moments beside the bus's own keys.
const keyPrefix = "gw:seen:"

// Store holds one moment per key until its time to live ends.
type Store struct{ rdb *redis.Client }

// New builds a store over rdb.
func New(rdb *redis.Client) Store { return Store{rdb: rdb} }

// Mark records at under key for ttl, replacing an earlier moment. A ttl under
// a second is raised to one, because Redis keeps a key with no expiry forever.
func (s Store) Mark(ctx context.Context, key string, at time.Time, ttl time.Duration) error {
	return s.rdb.Set(ctx, keyPrefix+key, at.UnixMilli(), max(ttl, time.Second)).Err()
}

// Last answers the moment under key, or the zero time when none is held.
func (s Store) Last(ctx context.Context, key string) (time.Time, error) {
	ms, err := s.rdb.Get(ctx, keyPrefix+key).Int64()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms), nil
}
