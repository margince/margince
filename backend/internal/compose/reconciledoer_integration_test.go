// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An approved follow-up is the deal owner's next step as the task is written:
// it reaches their own Worklist, not the shared unassigned queue. An owner who
// can no longer hold work leaves it unassigned rather than failing the approval.
func TestAnApprovedFollowUpIsTheDealOwnersOwnTask(t *testing.T) {
	for name, c := range map[string]struct {
		handOverTo func(e *reconcileEnv) *ids.UUID
		want       func(e *reconcileEnv) string
	}{
		"kept by the owner who approved": {
			handOverTo: func(*reconcileEnv) *ids.UUID { return nil },
			want:       func(e *reconcileEnv) string { return e.Rep1.String() },
		},
		"handed over before the task was written": {
			handOverTo: func(e *reconcileEnv) *ids.UUID { return &e.Rep2 },
			want:       func(e *reconcileEnv) string { return e.Rep2.String() },
		},
		"owner left before the task was written": {
			handOverTo: func(e *reconcileEnv) *ids.UUID {
				e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep2)
				return &e.Rep2
			},
			want: func(*reconcileEnv) string { return "" },
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupReconcile(t)
			deal := e.SeedDeal(t, "Owner follows up", e.pipeline, e.open, &e.Rep1)
			e.seedInteraction(t, deal, "call", "Discovery", 1)
			if err := e.reconcile(); err != nil {
				t.Fatal(err)
			}
			approvalID, proposal := e.followUpApproval(t, deal)
			// Written past the handover path, as a handover landing between
			// the decision and its effect leaves the deal.
			if to := c.handOverTo(e); to != nil {
				e.WsExec(t, `UPDATE deal SET owner_id = $2 WHERE id = $1`, deal, *to)
			}
			approver := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
			if _, err := e.svc.Decide(approver, approvalID, true, nil); err != nil {
				t.Fatalf("approve + effect: %v", err)
			}
			got := e.WsScalar(t, `SELECT coalesce(a.assignee_id::text, '') FROM activity a
				JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
				WHERE a.kind = 'task' AND a.source = 'overnight-reconcile'`, deal)
			if got != c.want(e) {
				t.Fatalf("the approved follow-up is assigned to %q, want %q", got, c.want(e))
			}
			if got == e.Rep1.String() {
				assertOnlyInMine(t, e, approver, proposal.DueDate)
			}
		})
	}
}

// assertOnlyInMine checks the reader's own task lane holds the follow-up and
// the unassigned queue does not.
func assertOnlyInMine(t *testing.T, e *reconcileEnv, reader context.Context, dueDate string) {
	t.Helper()
	due, err := time.Parse(time.DateOnly, dueDate)
	if err != nil {
		t.Fatal(err)
	}
	now, until := clocktest.Now(t).UTC(), due.Add(48*time.Hour)
	tasks := attentionTasks{store: e.Activities}
	for scope, want := range map[attention.TaskScope]int{attention.TasksMine: 1, attention.TasksUnassigned: 0} {
		got, err := tasks.CountOpenForViewer(reader, now, until, scope, ids.Nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("the %v lane counts %d, want %d", scope, got, want)
		}
	}
}
