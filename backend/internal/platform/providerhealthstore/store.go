// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package providerhealthstore keeps each AI provider's blocking status in
// Redis, so the api and the worker report one fact about a provider instead of
// each process's own. It is the platform half of the AI module's
// ProviderHealthStore seam; the module never sees the Redis client.
package providerhealthstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	keyPrefix = "ai:provider-health:"
	// keyTTL lets a status nobody refreshes or clears (a process that died
	// while the provider was down) disappear on its own.
	keyTTL    = time.Hour
	scanBatch = 100
)

// Redis reads and writes provider statuses in one Redis.
type Redis struct{ rdb *redis.Client }

// New wraps rdb. The client belongs to the caller, who closes it.
func New(rdb *redis.Client) *Redis { return &Redis{rdb: rdb} }

type record struct {
	Health     model.ProviderHealth `json:"health"`
	Since      time.Time            `json:"since"`
	RetryAfter time.Time            `json:"retry_after"`
}

// Publish records provider's status under the TTL, replacing what was there.
func (s *Redis) Publish(ctx context.Context, provider string, st model.ProviderHealthStatus) error {
	raw, err := json.Marshal(record{Health: st.Health, Since: st.Since, RetryAfter: st.RetryAfter})
	if err != nil {
		return fmt.Errorf("providerhealthstore: encoding %s: %w", provider, err)
	}
	return s.rdb.Set(ctx, keyPrefix+provider, raw, keyTTL).Err()
}

// Clear drops provider's status; clearing a provider with none is not an error.
func (s *Redis) Clear(ctx context.Context, provider string) error {
	return s.rdb.Del(ctx, keyPrefix+provider).Err()
}

// Load returns every recorded status by provider. A value that does not decode, or
// names no unhealthy state, is skipped rather than failing the read: one bad key must not hide the rest,
// and it expires with its TTL.
func (s *Redis) Load(ctx context.Context) (map[string]model.ProviderHealthStatus, error) {
	out := map[string]model.ProviderHealthStatus{}
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, keyPrefix+"*", scanBatch).Result()
		if err != nil {
			return nil, fmt.Errorf("providerhealthstore: listing keys: %w", err)
		}
		for _, key := range keys {
			st, ok, err := s.read(ctx, key)
			if err != nil {
				return nil, err
			}
			if ok {
				out[strings.TrimPrefix(key, keyPrefix)] = st
			}
		}
		if next == 0 {
			return out, nil
		}
		cursor = next
	}
}

func (s *Redis) read(ctx context.Context, key string) (model.ProviderHealthStatus, bool, error) {
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return model.ProviderHealthStatus{}, false, nil
	}
	if err != nil {
		return model.ProviderHealthStatus{}, false, fmt.Errorf("providerhealthstore: reading %s: %w", key, err)
	}
	var rec record
	if json.Unmarshal(raw, &rec) != nil || !(rec.Health == model.HealthDegraded || rec.Health.Blocking()) {
		return model.ProviderHealthStatus{}, false, nil
	}
	return model.ProviderHealthStatus{Health: rec.Health, Since: rec.Since, RetryAfter: rec.RetryAfter}, true, nil
}
