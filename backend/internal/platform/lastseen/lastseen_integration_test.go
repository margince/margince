// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package lastseen

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/redistest"
)

func TestTheLatestMarkIsWhatLastAnswers(t *testing.T) {
	store := New(redistest.Client(t))
	ctx := t.Context()
	if got, err := store.Last(ctx, "conn"); err != nil || !got.IsZero() {
		t.Fatalf("Last before any mark = %v, %v; want the zero time", got, err)
	}
	first := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{first, first.Add(time.Minute)} {
		if err := store.Mark(ctx, "conn", at, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := store.Last(ctx, "conn"); err != nil || !got.Equal(first.Add(time.Minute)) {
		t.Errorf("Last = %v, %v; want the second mark %v", got, err, first.Add(time.Minute))
	}
}

func TestAMarkEndsWithItsTimeToLive(t *testing.T) {
	store := New(redistest.Client(t))
	ctx := t.Context()
	if err := store.Mark(ctx, "conn", clocktest.Now(t), time.Second); err != nil {
		t.Fatal(err)
	}
	ttl, err := store.rdb.TTL(ctx, keyPrefix+"conn").Result()
	if err != nil || ttl <= 0 || ttl > time.Second {
		t.Errorf("the mark lives %v (%v); want at most the second it was given", ttl, err)
	}
}
