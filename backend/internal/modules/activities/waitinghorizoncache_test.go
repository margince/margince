// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var horizonNoon = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// The point of the cache: inside the hour the owed pass, the queue and the
// guardrail stop re-measuring a year of answers. Handed back unchanged, for the
// asOf it was measured at and for one a few minutes on.
func TestARememberedHorizonIsReusedWithinTheHour(t *testing.T) {
	cache := newHorizonCache()
	ws := ids.New[ids.WorkspaceKind]()
	cache.remember(ws, horizonNoon, horizonNoon, 42)

	for _, later := range []time.Duration{0, time.Minute, waitingHorizonTTL - time.Second} {
		got, ok := cache.lookup(ws, horizonNoon.Add(later), horizonNoon.Add(later))
		if !ok || got != 42 {
			t.Errorf("%s after measuring, the lookup answered (%d, %v), want the remembered 42", later, got, ok)
		}
	}
}

// Each way a remembered horizon stops being the answer. An hour old is the TTL;
// a different asOf is a different question of a year-long window, even when
// the clock has not moved (a suite pinning it, a report over another quarter);
// a clock that runs backwards is not a very fresh entry; and another
// workspace never reads this one's number.
func TestARememberedHorizonIsNotReusedPastWhatItAnswered(t *testing.T) {
	ws := ids.New[ids.WorkspaceKind]()
	cases := map[string]struct {
		ws        ids.WorkspaceID
		asOf, now time.Time
	}{
		"an hour on":          {ws, horizonNoon.Add(waitingHorizonTTL), horizonNoon.Add(waitingHorizonTTL)},
		"another asOf":        {ws, horizonNoon.AddDate(0, -3, 0), horizonNoon},
		"a clock run back":    {ws, horizonNoon, horizonNoon.Add(-time.Minute)},
		"another workspace":   {ids.New[ids.WorkspaceKind](), horizonNoon, horizonNoon},
		"a later asOf, stale": {ws, horizonNoon.Add(2 * waitingHorizonTTL), horizonNoon.Add(time.Minute)},
	}
	for name, c := range cases {
		cache := newHorizonCache()
		cache.remember(ws, horizonNoon, horizonNoon, 42)
		if got, ok := cache.lookup(c.ws, c.asOf, c.now); ok {
			t.Errorf("%s: the lookup reused %d, want a miss that measures again", name, got)
		}
	}
}

// A store assembled bare, without NewStore, has no cache — and must measure
// rather than panic, which is what every such store did before it existed.
func TestANilHorizonCacheAlwaysMisses(t *testing.T) {
	var cache *horizonCache
	cache.remember(ids.New[ids.WorkspaceKind](), horizonNoon, horizonNoon, 42)
	if got, ok := cache.lookup(ids.New[ids.WorkspaceKind](), horizonNoon, horizonNoon); ok {
		t.Fatalf("a nil cache answered %d, want a miss", got)
	}
}

// The wiring, not just the map: waitingHorizonFor answers a remembered horizon
// without touching the transaction at all. A nil tx is the proof — a
// measurement would dereference it.
func TestTheStoreServesTheRememberedHorizonWithoutMeasuring(t *testing.T) {
	ws := ids.New[ids.WorkspaceKind]()
	store := NewStore(database.BindTo(nil, ws)).WithClock(func() time.Time { return horizonNoon })
	store.horizons.remember(ws, horizonNoon, horizonNoon, 42)

	got, err := store.waitingHorizonFor(context.Background(), nil, horizonNoon)
	if err != nil || got != 42 {
		t.Fatalf("waitingHorizonFor answered (%d, %v), want the remembered 42 with no measurement", got, err)
	}
}
