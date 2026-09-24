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

// A reply cut off at the output ceiling, or one with no text, is one bad roll
// of the model. The cache itself refuses it, so no writer can replay it by
// forgetting to check; every other terminal is cacheable, whichever wire spelled
// it.
func TestResultCacheKeepsOnlyAnswersWorthReplaying(t *testing.T) {
	for name, tc := range map[string]struct {
		resp model.Response
		kept bool
	}{
		"a finished reply":                    {resp: model.Response{Text: "whole", FinishReason: "stop"}, kept: true},
		"a finished reply in anthropic terms": {resp: model.Response{Text: "whole", FinishReason: "end_turn"}, kept: true},
		"a finished reply in gemini terms":    {resp: model.Response{Text: "whole", FinishReason: "STOP"}, kept: true},
		"a reply naming no terminal":          {resp: model.Response{Text: "whole"}, kept: true},
		"a reply cut off at the ceiling":      {resp: model.Response{Text: `{"answer":"aa`, FinishReason: model.FinishReasonLength}},
		"an empty reply":                      {resp: model.Response{FinishReason: "stop"}},
		"a reply of only whitespace":          {resp: model.Response{Text: " \n\t", FinishReason: "stop"}},
	} {
		t.Run(name, func(t *testing.T) {
			c := newResultCache(time.Minute)
			ws := ids.New[ids.WorkspaceKind]()
			c.put("key", ws, sameBinding, tc.resp, TierCheapCloud)
			if _, _, hit := c.get("key", ws, sameBinding); hit != tc.kept {
				t.Errorf("cached = %v, want %v", hit, tc.kept)
			}
		})
	}
}

// What the rule is for, through the router: the identical next request reaches
// the model again instead of being served the cut-off answer for the TTL.
func TestTheRouterDoesNotReplayACutOffAnswer(t *testing.T) {
	cheap := NewFakeClient().ScriptSteps(
		FakeStep{Text: `{"answer":"aa`, FinishReason: model.FinishReasonLength},
		FakeStep{Text: `{"answer":"whole"}`, FinishReason: "stop"},
	)
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
	ctx := wsContext(t)
	req := model.Request{Messages: []model.Message{{Role: "user", Content: "same thread"}}}

	if _, _, err := r.Complete(ctx, TaskSummarize, req); err != nil {
		t.Fatal(err)
	}
	second, info, err := r.Complete(ctx, TaskSummarize, req)
	if err != nil {
		t.Fatal(err)
	}
	if info.Cached || second.Text != `{"answer":"whole"}` {
		t.Fatalf("the second request was served %q (cached %v), want the fresh whole answer", second.Text, info.Cached)
	}
	if len(cheap.Calls()) != 2 {
		t.Fatalf("model called %d times, want 2 — the cut-off answer must not stand in for a fresh call", len(cheap.Calls()))
	}
}
