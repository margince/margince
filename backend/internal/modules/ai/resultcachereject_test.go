// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func rejectReq() model.Request {
	return model.Request{Messages: []model.Message{{Role: "user", Content: "propose the next step"}}}
}

// An answer the caller refused is one bad roll of the model; the identical next
// request must reach the model rather than be served the refusal for the TTL.
func TestARejectedAnswerReachesTheModelAgain(t *testing.T) {
	cheap := NewFakeClient().Script("not a step", `{"final":{}}`)
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
	ctx := wsContext(t)

	_, info, err := r.Complete(ctx, TaskSummarize, rejectReq())
	if err != nil {
		t.Fatal(err)
	}
	r.Reject(info)

	resp, again, err := r.Complete(ctx, TaskSummarize, rejectReq())
	if err != nil {
		t.Fatal(err)
	}
	if again.Cached || resp.Text != `{"final":{}}` {
		t.Fatalf("served %q (cached %v), want the fresh answer", resp.Text, again.Cached)
	}
	if calls := len(cheap.Calls()); calls != 2 {
		t.Fatalf("model called %d times, want 2", calls)
	}
}

// The control: an answer nobody rejected is still served from the cache.
func TestAnAnswerNobodyRejectedIsStillReplayed(t *testing.T) {
	cheap := NewFakeClient().Script(`{"final":{}}`)
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
	ctx := wsContext(t)

	if _, _, err := r.Complete(ctx, TaskSummarize, rejectReq()); err != nil {
		t.Fatal(err)
	}
	_, info, err := r.Complete(ctx, TaskSummarize, rejectReq())
	if err != nil {
		t.Fatal(err)
	}
	if !info.Cached || len(cheap.Calls()) != 1 {
		t.Fatalf("cached %v after %d model calls, want a cache hit after 1", info.Cached, len(cheap.Calls()))
	}
}

// A rejection is one workspace's verdict on its own answer: the identical
// request in another workspace keeps its cached answer.
func TestARejectionLeavesAnotherWorkspacesAnswerCached(t *testing.T) {
	cheap := NewFakeClient().Script("refused here", "accepted there", "fresh")
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
	rejecting, other := wsContext(t), wsContext(t)

	_, info, err := r.Complete(rejecting, TaskSummarize, rejectReq())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Complete(other, TaskSummarize, rejectReq()); err != nil {
		t.Fatal(err)
	}
	r.Reject(info)

	resp, kept, err := r.Complete(other, TaskSummarize, rejectReq())
	if err != nil {
		t.Fatal(err)
	}
	if !kept.Cached || resp.Text != "accepted there" {
		t.Fatalf("the other workspace was served %q (cached %v), want its own cached answer", resp.Text, kept.Cached)
	}
	if _, fresh, err := r.Complete(rejecting, TaskSummarize, rejectReq()); err != nil || fresh.Cached {
		t.Fatalf("the rejecting workspace was served from cache (%v)", err)
	}
}

// A RouteInfo that names no served answer, such as a failed call's, evicts nothing.
func TestRejectingAnUnservedCallEvictsNothing(t *testing.T) {
	r := testRouter(map[Tier]model.Client{TierCheapCloud: NewFakeClient()}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
	ws := ids.From[ids.WorkspaceKind](ids.NewV7())
	key, err := cacheKey(ws, TaskSummarize, rejectReq())
	if err != nil {
		t.Fatal(err)
	}
	r.cache.put(key, ws, r.binding().generation, model.Response{Text: "kept"}, TierCheapCloud)

	_, info, err := r.Complete(context.Background(), TaskSummarize, rejectReq())
	if err == nil {
		t.Fatal("a call outside a workspace must fail")
	}
	r.Reject(info)

	if _, _, ok := r.cache.get(key, ws, r.binding().generation); !ok {
		t.Fatal("rejecting a call that served nothing dropped a cached answer")
	}
}
