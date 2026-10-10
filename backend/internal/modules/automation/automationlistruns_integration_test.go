// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

// The automations list's run summary over a real migrated Postgres. Every run
// is written by the engine itself, a retry included, so the summary is proven
// against the key shape runKey actually writes.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

const summarizedHandler = "summarized_rule"

// summarizedRuns drives two rules of one handler through the engine: both
// fail once, only the first is retried (and applies), then both skip. The
// first rule ends with two counted runs, the second with one.
func summarizedRuns(t *testing.T, fx *autoFixture) (rule, sibling ids.AutomationID) {
	t.Helper()
	rule = fx.seedAutomation(t, summarizedHandler)
	sibling = fx.seedAutomation(t, summarizedHandler)
	skipEntity := ids.NewV7()
	engine := engineOverScripted(fx, scriptedWorkflow{
		name: summarizedHandler, redrivable: true,
		match: func(ev workflow.Event) (bool, error) { return ev.Entity.ID != skipEntity, nil },
		apply: func(ev workflow.Event) (workflow.RunResult, error) {
			if ev.RetryAttempt == 0 {
				return workflow.RunResult{}, errors.New("the downstream service was unreachable")
			}
			return workflow.RunResult{}, nil
		},
	})
	if err := engine.HandleEvent(context.Background(), fx.stagedEvent(t, ids.NewV7())); err == nil {
		t.Fatal("the scripted Apply failed, so HandleEvent must surface it")
	}
	failedID, _ := fx.runOf(t, rule, "failed")
	if outcome, err := engine.RetryRun(context.Background(), failedID); err != nil || !outcome.Retried {
		t.Fatalf("retrying the rule's failed run answered (%+v, %v), want a retry", outcome, err)
	}
	if err := engine.HandleEvent(context.Background(), fx.stagedEvent(t, skipEntity)); err != nil {
		t.Fatalf("a declined trigger is a skip, not an error: %v", err)
	}
	return rule, sibling
}

// runOf reads the one run of a status the automation recorded.
func (fx *autoFixture) runOf(t *testing.T, automationID ids.AutomationID, status string) (ids.UUID, time.Time) {
	t.Helper()
	var id ids.UUID
	var at time.Time
	if err := fx.owner.QueryRow(context.Background(), `
		SELECT id, created_at FROM workflow_run
		 WHERE right(idempotency_key, 36) = $1 AND status = $2`,
		automationID.String(), status).Scan(&id, &at); err != nil {
		t.Fatalf("reading the %s run of %s: %v", status, automationID, err)
	}
	return id, at
}

// listedRuns pages the whole list one row at a time, so the window's
// placeholder is proven beside the cursor's, and keys each summary by rule.
func listedRuns(ctx context.Context, t *testing.T, store *AutomationStore) map[ids.AutomationID]RunSummary {
	t.Helper()
	limit := 1
	out := map[ids.AutomationID]RunSummary{}
	var cursor *string
	for {
		page, err := store.List(ctx, cursor, &limit)
		if err != nil {
			t.Fatalf("listing automations: %v", err)
		}
		for _, a := range page.Items {
			if a.Runs == nil {
				t.Fatalf("listed automation %s carries no run summary", a.ID)
			}
			out[a.ID] = *a.Runs
		}
		if !page.HasMore {
			return out
		}
		cursor = &page.NextCursor
	}
}

func lastOutcome(runs RunSummary) string {
	if outcome := runs.LastOutcome(); outcome != nil {
		return *outcome
	}
	return "none"
}

func TestTheListSummarizesEachRulesOwnCountedRuns(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	rule, sibling := summarizedRuns(t, fx)
	quiet := fx.seedAutomation(t, "quiet_rule")
	_, retriedAt := fx.runOf(t, rule, "applied")
	_, siblingFailedAt := fx.runOf(t, sibling, "failed")
	store := NewAutomationStore(db).WithClock(func() time.Time { return retriedAt })
	ctx := fx.humanCtx(fx.rep1, principal.RowScopeAll)

	runs := listedRuns(ctx, t, store)
	if len(runs) != 3 {
		t.Fatalf("listed %d automations, want 3", len(runs))
	}
	got := runs[rule]
	if got.RecentRuns != 2 || got.LastRunAt == nil || !got.LastRunAt.Equal(retriedAt) || lastOutcome(got) != "fired" {
		t.Errorf("the retried rule reads %d runs, last %v %s; want 2, the retry at %v, fired",
			got.RecentRuns, got.LastRunAt, lastOutcome(got), retriedAt)
	}
	got = runs[sibling]
	if got.RecentRuns != 1 || got.LastRunAt == nil || !got.LastRunAt.Equal(siblingFailedAt) || lastOutcome(got) != "failed" {
		t.Errorf("the sibling of one handler reads %d runs, last %v %s; want only its own failure",
			got.RecentRuns, got.LastRunAt, lastOutcome(got))
	}
	if got = runs[quiet]; got.RecentRuns != 0 || got.LastRunAt != nil || lastOutcome(got) != "none" {
		t.Errorf("a rule that never ran reads %+v, want no last run and 0", got)
	}
	if single, err := store.Get(ctx, rule); err != nil || single.Runs != nil {
		t.Errorf("a single read carried a run summary (%v, %v); only the list reads one", single.Runs, err)
	}
}

// The window starts 30 days before the server's clock and includes its first
// instant; the last run is the last one ever, whatever the window.
func TestTheListCountsRunsInsideTheWindowOnly(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	rule, _ := summarizedRuns(t, fx)
	_, failedAt := fx.runOf(t, rule, "failed")
	_, retriedAt := fx.runOf(t, rule, "applied")
	ctx := fx.humanCtx(fx.rep1, principal.RowScopeAll)

	cases := []struct {
		name string
		now  time.Time
		want int
	}{
		{"the failure sits on the window's first instant", failedAt.Add(recentRunsWindow), 2},
		{"the failure is a microsecond too old", failedAt.Add(recentRunsWindow + time.Microsecond), 1},
		{"both runs are too old", retriedAt.Add(recentRunsWindow + time.Microsecond), 0},
	}
	for _, c := range cases {
		store := NewAutomationStore(db).WithClock(func() time.Time { return c.now })
		got := listedRuns(ctx, t, store)[rule]
		if got.RecentRuns != c.want || got.LastRunAt == nil || !got.LastRunAt.Equal(retriedAt) {
			t.Errorf("%s: %d runs, last %v; want %d, last still the retry at %v",
				c.name, got.RecentRuns, got.LastRunAt, c.want, retriedAt)
		}
	}
}

func TestTheListWithItsRunSummaryNeedsAutomationRead(t *testing.T) {
	fx := setupAutomationDB(t)
	summarizedRuns(t, fx)
	store := NewAutomationStore(database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws)))
	ctx := principal.WithWorkspaceID(context.Background(), fx.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + fx.rep2.String(), UserID: fx.rep2,
		Permissions: principal.Permissions{
			RoleKeys: []string{"test"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true}},
		},
	})
	if _, err := store.List(ctx, nil, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("listing without automation read → %v, want ErrPermissionDenied", err)
	}
}
