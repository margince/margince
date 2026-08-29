// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The troubled read behind the ai_work_health lane, against real rows written
// by ApplyStateChange — the projection's only writer. The stalled arm is SQL
// against the database clock and the window predicate is SQL against
// finished_at, so a unit double proves neither.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTroubledCarriesFailedAndStalledRunsAndOnlyTheCallers(t *testing.T) {
	a := newAIActivityEnv(t)
	// One failed run for Rep1.
	a.apply(t, change{attempt: 1, state: "running"})
	a.apply(t, change{attempt: 1, state: "failed", degradeReason: "the model refused"})

	// One stalled run for Rep1: live, with a lease already past. StaleAfter
	// compares against the database's own now() at read time.
	stalledKey := "attachment_extraction:" + ids.NewV7().String()
	stale := a.queuedAt.Add(-time.Minute)
	started := a.queuedAt.Add(-2 * time.Minute)
	if _, err := a.store.ApplyStateChange(a.env.Admin(), aiactivity.Change{
		Source: "attachment_extraction", OccurrenceKey: stalledKey,
		Kind: "document_extract", AITask: "document_extract", Attempt: 1,
		ActorScope: aiactivity.ScopePersonal, ActorUserID: a.env.Rep1,
		State: "running", QueuedAt: started, StartedAt: &started, StaleAfter: &stale,
		SubjectLabel: "Weber GmbH", EventID: ids.NewV7(),
	}); err != nil {
		t.Fatalf("projecting the stalled run: %v", err)
	}

	// A second person's failed AND stalled runs must never reach Rep1's lane
	// — one negative row per UNION arm, so dropping the principal predicate
	// from either arm fails here rather than leaking live work.
	if _, err := a.store.ApplyStateChange(a.env.Admin(), aiactivity.Change{
		Source: "attachment_extraction", OccurrenceKey: "attachment_extraction:" + ids.NewV7().String(),
		Kind: "document_extract", AITask: "document_extract", Attempt: 1,
		ActorScope: aiactivity.ScopePersonal, ActorUserID: a.env.Rep2,
		State: "failed", QueuedAt: a.queuedAt, StartedAt: &a.queuedAt, FinishedAt: &a.queuedAt,
		EventID: ids.NewV7(),
	}); err != nil {
		t.Fatalf("projecting the other person's failed run: %v", err)
	}
	if _, err := a.store.ApplyStateChange(a.env.Admin(), aiactivity.Change{
		Source: "attachment_extraction", OccurrenceKey: "attachment_extraction:" + ids.NewV7().String(),
		Kind: "document_extract", AITask: "document_extract", Attempt: 1,
		ActorScope: aiactivity.ScopePersonal, ActorUserID: a.env.Rep2,
		State: "running", QueuedAt: started, StartedAt: &started, StaleAfter: &stale,
		EventID: ids.NewV7(),
	}); err != nil {
		t.Fatalf("projecting the other person's stalled run: %v", err)
	}

	rep1 := a.env.As(a.env.Rep1, []ids.UUID{a.env.Team1}, AccountRepPerms)
	troubled, err := a.store.Troubled(rep1, a.queuedAt.Add(-24*time.Hour), 8)
	if err != nil {
		t.Fatalf("Troubled: %v", err)
	}
	if len(troubled) != 2 {
		t.Fatalf("Troubled = %+v, want exactly Rep1's failed and stalled runs", troubled)
	}
	// The outer ORDER BY is what makes "stalled first" true rather than an
	// accident of UNION ALL, so it is asserted where real SQL runs.
	if troubled[0].State != aiactivity.StateStalled || troubled[1].State != "failed" {
		t.Fatalf("Troubled order = [%s, %s], want the stalled run first", troubled[0].State, troubled[1].State)
	}

	// The window is real: a read whose window opens after the failure keeps
	// the stalled run (no window applies to a live row) and drops the failed.
	narrow, err := a.store.Troubled(rep1, a.queuedAt.Add(time.Hour), 8)
	if err != nil {
		t.Fatalf("Troubled with a later window: %v", err)
	}
	if len(narrow) != 1 || narrow[0].State != aiactivity.StateStalled {
		t.Fatalf("narrow window = %+v, want only the stalled run", narrow)
	}
}
