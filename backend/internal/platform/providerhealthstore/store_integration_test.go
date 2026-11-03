// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package providerhealthstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAStatusPublishedByOneProcessIsLoadedByAnotherAndExpires(t *testing.T) {
	rdb := redistest.Client(t)
	writer, reader := New(rdb), New(rdb)
	since := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	down := model.ProviderHealthStatus{Health: model.HealthDown, Since: since, RetryAfter: since.Add(time.Minute)}

	if err := writer.Publish(t.Context(), "openai", down); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// A provider that is indexed but whose value is garbled, and one whose
	// key has expired: a read skips both without changing stored evidence.
	if err := rdb.Set(t.Context(), keyPrefix+"garbled", "{not json", keyTTL).Err(); err != nil {
		t.Fatalf("seeding a garbled value: %v", err)
	}
	if err := rdb.SAdd(t.Context(), indexKey, "garbled", "expired").Err(); err != nil {
		t.Fatalf("indexing the seeded providers: %v", err)
	}
	got, err := reader.Load(t.Context())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || !got["openai"].Since.Equal(since) || got["openai"].Health != model.HealthDown ||
		!got["openai"].RetryAfter.Equal(down.RetryAfter) {
		t.Fatalf("loaded %+v, want only openai %+v", got, down)
	}
	if members := rdb.SMembers(t.Context(), indexKey).Val(); len(members) != 3 {
		t.Errorf("index %v after a load, want the read to leave all three indexed providers unchanged", members)
	}
	if err := writer.Publish(t.Context(), "openai", down); err != nil {
		t.Fatal(err)
	}
	if exists := rdb.SIsMember(t.Context(), indexKey, "expired").Val(); exists {
		t.Fatal("writer left an expired provider in the index")
	}
	if ttl := rdb.TTL(t.Context(), keyPrefix+"openai").Val(); ttl <= 0 || ttl > keyTTL {
		t.Errorf("ttl = %v, want within (0, %v]", ttl, keyTTL)
	}

	if err := reader.Clear(t.Context(), "openai"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if err := reader.Clear(t.Context(), "openai"); err != nil {
		t.Fatalf("clearing an absent key must not fail: %v", err)
	}
	if got, err := writer.Load(t.Context()); err != nil || len(got) != 0 {
		t.Fatalf("after clear: %v, %v", got, err)
	}
}

type failIndexRead struct{}

func (failIndexRead) DialHook(next redis.DialHook) redis.DialHook { return next }
func (failIndexRead) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (failIndexRead) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		if command.Name() == "smembers" {
			return errors.New("synthetic index read unavailable")
		}
		return next(ctx, command)
	}
}

func TestIndexCleanupFailureDoesNotUndoRecordedStatus(t *testing.T) {
	rdb := redistest.Client(t)
	rdb.AddHook(failIndexRead{})
	store := New(rdb)
	status := model.ProviderHealthStatus{Health: model.HealthDown, Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	if err := store.Publish(t.Context(), "fixture", status); err != nil {
		t.Fatalf("recorded status reported failed: %v", err)
	}
	if raw, err := rdb.Get(t.Context(), keyPrefix+"fixture").Result(); err != nil || raw == "" {
		t.Fatalf("status not recorded: %q %v", raw, err)
	}
	if err := store.Clear(t.Context(), "fixture"); err != nil {
		t.Fatalf("cleared status reported failed: %v", err)
	}
	if count, err := rdb.Exists(t.Context(), keyPrefix+"fixture").Result(); err != nil || count != 0 {
		t.Fatalf("status not cleared: %d %v", count, err)
	}
}
