// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// TestResultCacheNeverExceedsCapacity: expired entries are reaped lazily
// on same-key reads only, so the capacity bound is what keeps a stream of
// unique requests from growing process memory without limit.
// sameBinding: these cases are about expiry and the size cap, so every entry
// belongs to one binding and the generation never varies.
const sameBinding uint64 = 1

func TestResultCacheNeverExceedsCapacity(t *testing.T) {
	c := newResultCache(time.Minute)
	ws := ids.New[ids.WorkspaceKind]()
	for i := 0; i < maxResultCacheEntries+50; i++ {
		c.put(fmt.Sprintf("key-%d", i), ws, sameBinding, model.Response{Text: "r"}, TierCheapCloud)
	}
	if len(c.entries) > maxResultCacheEntries {
		t.Fatalf("cache holds %d entries, over the %d cap", len(c.entries), maxResultCacheEntries)
	}
}

// TestResultCacheEvictsExpiredBeforeLive: when the cap forces eviction,
// dead entries go first — a full cache of expired residue must not cost
// a live answer its slot.
func TestResultCacheEvictsExpiredBeforeLive(t *testing.T) {
	c := newResultCache(time.Minute)
	ws := ids.New[ids.WorkspaceKind]()
	current := time.Unix(0, 0)
	c.now = func() time.Time { return current }

	for i := 0; i < maxResultCacheEntries-1; i++ {
		c.put(fmt.Sprintf("stale-%d", i), ws, sameBinding, model.Response{Text: "old"}, TierCheapCloud)
	}
	current = current.Add(2 * time.Minute) // everything above is now expired
	c.put("live-1", ws, sameBinding, model.Response{Text: "fresh"}, TierCheapCloud)
	c.put("live-2", ws, sameBinding, model.Response{Text: "fresh"}, TierCheapCloud) // at cap: triggers the sweep

	if _, _, ok := c.get("live-1", ws, sameBinding); !ok {
		t.Fatal("live entry lost while expired residue occupied the cache")
	}
	if _, _, ok := c.get("stale-0", ws, sameBinding); ok {
		t.Fatal("expired entry survived the capacity sweep")
	}
	if len(c.entries) > maxResultCacheEntries {
		t.Fatalf("cache holds %d entries, over the %d cap", len(c.entries), maxResultCacheEntries)
	}
}
