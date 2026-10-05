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
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	keyPrefix = "ai:provider-health:"
	// indexKey lists the providers with a status, so Load never scans.
	indexKey = "ai:provider-health-index"
	// keyTTL lets a status nobody refreshes or clears (a process that died
	// while the provider was down) disappear on its own.
	keyTTL = 30 * time.Minute
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

// Publish records provider's status under the TTL, replacing what was there,
// and files the provider in the index Load reads.
func (s *Redis) Publish(ctx context.Context, provider string, st model.ProviderHealthStatus) error {
	raw, err := json.Marshal(record{Health: st.Health, Since: st.Since, RetryAfter: st.RetryAfter})
	if err != nil {
		return fmt.Errorf("providerhealthstore: encoding %s: %w", provider, err)
	}
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, keyPrefix+provider, raw, keyTTL)
	pipe.SAdd(ctx, indexKey, provider)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("providerhealthstore: publishing %s: %w", provider, err)
	}
	return nil
}

// Clear drops provider's status; clearing a provider with none is not an error.
func (s *Redis) Clear(ctx context.Context, provider string) error {
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, keyPrefix+provider)
	pipe.SRem(ctx, indexKey, provider)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("providerhealthstore: clearing %s: %w", provider, err)
	}
	return nil
}

// Load returns every recorded unhealthy status by provider. It reads the index
// of providers and then their values, never scanning the keyspace, which a busy
// installation shares with far larger key families. A value that has expired,
// does not decode, or names no unhealthy state is skipped rather than failing
// the read: one bad key must not hide the rest.
func (s *Redis) Load(ctx context.Context) (map[string]model.ProviderHealthStatus, error) {
	providers, err := s.rdb.SMembers(ctx, indexKey).Result()
	if err != nil {
		return nil, fmt.Errorf("providerhealthstore: listing providers: %w", err)
	}
	out := map[string]model.ProviderHealthStatus{}
	if len(providers) == 0 {
		return out, nil
	}
	keys := make([]string, len(providers))
	for i, provider := range providers {
		keys[i] = keyPrefix + provider
	}
	values, err := s.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("providerhealthstore: reading statuses: %w", err)
	}
	var expired []any
	for i, value := range values {
		raw, ok := value.(string)
		if !ok {
			expired = append(expired, providers[i])
			continue
		}
		var rec record
		if json.Unmarshal([]byte(raw), &rec) != nil || (rec.Health != model.HealthDegraded && !rec.Health.Blocking()) {
			continue
		}
		out[providers[i]] = model.ProviderHealthStatus{Health: rec.Health, Since: rec.Since, RetryAfter: rec.RetryAfter}
	}
	if len(expired) > 0 {
		// A key that expired leaves its provider in the index; drop it so the
		// index does not grow past what is recorded. A failure is not the
		// reader's problem: the next Load tries again.
		for _, provider := range expired {
			if err := s.pruneIfStillAbsent(ctx, provider.(string)); err != nil {
				slog.WarnContext(ctx, "providerhealthstore: pruning an expired provider from the index", "provider", provider, "error", err)
			}
		}
	}
	return out, nil
}

// pruneScript drops a provider from the index only while its key is still
// absent, in one step: a Publish that landed between the read that saw the key
// expired and this removal has written the key again, and must keep its place.
var pruneScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[2]) == 0 then
  return redis.call('SREM', KEYS[1], ARGV[1])
end
return 0`)

func (s *Redis) pruneIfStillAbsent(ctx context.Context, provider string) error {
	return pruneScript.Run(ctx, s.rdb, []string{indexKey, keyPrefix + provider}, provider).Err()
}
