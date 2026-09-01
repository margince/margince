// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

// The cross-instance troubled-runs read: only failed and blocked firings,
// only live automations, only the window — against rows in the exact shape
// the engine writes (the shared seedRun fixture spells the handler and
// idempotency-key linkage the way engine_run.go does).

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTroubledRunsCarriesFailedAndBlockedAcrossLiveRulesOnly(t *testing.T) {
	fx := setupAutomationDB(t)
	store := NewAutomationStore(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)))
	ctx := fx.humanCtx(fx.rep1, principal.RowScopeAll)
	autoID := fx.seedAutomation(t, "stage_change_create_task")
	seedRunHistory(t, fx, autoID) // one run per terminal status, minutes apart

	// An archived rule's failures are history, not a card: nobody can open
	// the rule the card would name.
	archivedID := fx.seedAutomation(t, "stage_change_create_task")
	fx.seedRun(t, archivedID, "stage_change_create_task", "failed", nil,
		time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC))
	fx.exec(t, `UPDATE automation SET archived_at = now() WHERE id = $1`, archivedID)

	// A PAUSED rule's failures raise nothing either: its owner turned it
	// off, and the card would nag them about their own decision.
	pausedID := fx.seedAutomation(t, "stage_change_create_task")
	fx.seedRun(t, pausedID, "stage_change_create_task", "failed", nil,
		time.Date(2026, 6, 1, 13, 30, 0, 0, time.UTC))
	fx.exec(t, `UPDATE automation SET enabled = false WHERE id = $1`, pausedID)

	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	troubled, err := store.TroubledRuns(ctx, since, 8)
	if err != nil {
		t.Fatalf("TroubledRuns: %v", err)
	}
	if len(troubled) != 2 {
		t.Fatalf("TroubledRuns = %+v, want exactly the live rule's failed and blocked firings", troubled)
	}
	outcomes := map[string]string{}
	for _, run := range troubled {
		outcomes[run.Outcome] = stringOrEmpty(run.Reason)
		if run.Name != "stage_change_create_task" {
			t.Errorf("run names rule %q, want the automation's own name", run.Name)
		}
		// The RULE, not this firing of it: the identity two failures of one
		// broken rule share, and which the worklist folds them on. A name is
		// mutable and not unique, so it cannot carry this.
		if run.AutomationID != autoID {
			t.Errorf("run names rule id %v, want the live automation %v", run.AutomationID, autoID)
		}
	}
	if outcomes["failed"] != "provider error" || outcomes["blocked"] != "approval rejected" {
		t.Fatalf("outcomes = %v, want failed/blocked with the engine's own reasons", outcomes)
	}

	// The window is real: a read opening after every firing carries nothing.
	late, err := store.TroubledRuns(ctx, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 8)
	if err != nil {
		t.Fatalf("TroubledRuns with a late window: %v", err)
	}
	if len(late) != 0 {
		t.Fatalf("late window = %+v, want empty", late)
	}
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
