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

// An approved follow-up is the deal owner's next step: it names them as its
// doer and reaches their own Worklist, not the shared unassigned queue.
func TestAnApprovedFollowUpIsTheDealOwnersOwnTask(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Owner follows up", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, proposal := e.followUpApproval(t, deal)
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(owner, approvalID, true, nil); err != nil {
		t.Fatalf("approve + effect: %v", err)
	}

	var assignee *ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT a.assignee_id FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		WHERE a.kind = 'task' AND a.source = 'overnight-reconcile'`, deal).Scan(&assignee); err != nil {
		t.Fatalf("reading the follow-up task: %v", err)
	}
	if assignee == nil || *assignee != e.Rep1 {
		t.Fatalf("the approved follow-up is assigned to %v, want the deal owner %s", assignee, e.Rep1)
	}

	due, err := time.Parse(time.DateOnly, proposal.DueDate)
	if err != nil {
		t.Fatal(err)
	}
	now := clocktest.Now(t).UTC()
	until := due.Add(48 * time.Hour)
	tasks := attentionTasks{store: e.Activities}
	mine, err := tasks.CountOpenForViewer(owner, now, until, attention.TasksMine, ids.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if mine != 1 {
		t.Errorf("the deal owner's own task lane counts %d, want the approved follow-up", mine)
	}
	unassigned, err := tasks.CountOpenForViewer(owner, now, until, attention.TasksUnassigned, ids.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if unassigned != 0 {
		t.Errorf("the unassigned queue counts %d, want none: the follow-up has a doer", unassigned)
	}
}
