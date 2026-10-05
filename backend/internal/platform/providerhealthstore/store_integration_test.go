// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package providerhealthstore

import (
	"testing"
	"time"

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
	if err := rdb.Set(t.Context(), keyPrefix+"garbled", "{not json", keyTTL).Err(); err != nil {
		t.Fatalf("seeding a garbled value: %v", err)
	}
	got, err := reader.Load(t.Context())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 1 || !got["openai"].Since.Equal(since) || got["openai"].Health != model.HealthDown ||
		!got["openai"].RetryAfter.Equal(down.RetryAfter) {
		t.Fatalf("loaded %+v, want only openai %+v", got, down)
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
