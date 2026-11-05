// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func assertRepairDecisionProjection(ctx context.Context, t *testing.T, e *integration.Env, at time.Time, want int) {
	t.Helper()
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	home, err := feed.Assemble(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if home.Counts.NeedsYou != want || len(home.NeedsYou) != want {
		t.Fatalf("Home decisions count=%d rows=%d, want %d", home.Counts.NeedsYou, len(home.NeedsYou), want)
	}
	worklist, err := feed.Worklist(ctx, "mine", "decisions", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, count := range worklist.Counts {
		if count.Category != "decisions" {
			continue
		}
		found = true
		if count.Considered != want || count.Shown != want || count.MoreAvailable {
			t.Fatalf("Worklist decision count=%+v, want %d complete decisions", count, want)
		}
	}
	if !found {
		t.Fatal("Worklist omitted the decision count")
	}
	shown := 0
	for _, item := range worklist.Queue {
		if item.Category != "decisions" {
			t.Fatalf("decision filter admitted %s", item.Category)
		}
		if item.Batch != nil {
			shown += item.Batch.Count
		} else {
			shown++
		}
	}
	if shown != want {
		t.Fatalf("Worklist grouped rows represent %d decisions, want %d", shown, want)
	}
}
