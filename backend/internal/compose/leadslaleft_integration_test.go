// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// The event is redelivered by an at-least-once bus, so the target it names can
// leave the company between the scan and the delivery. The breach is already
// marked escalated, so a refused write here would lose it for good.
func TestAnEscalationWhoseTargetLeftStillWritesItsTask(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	lead, ev, deadline, ctx := breachedLeadEvent(t, e, owner)
	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1); err != nil {
		t.Fatal(err)
	}
	h := newLeadSLAEscalation(e.DB(), func() time.Time { return deadline.Add(time.Hour) })
	eff, err := h.Plan(ctx, ev)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := h.Apply(ctx, ev, eff, nil); err != nil {
		t.Fatalf("apply for a target who has left: %v", err)
	}

	tasks, assignee := openTasksOnLead(t, owner, lead)
	if tasks != 1 {
		t.Fatalf("open tasks on the lead = %d, want 1", tasks)
	}
	if assignee != nil {
		t.Errorf("the task was assigned to %s, a seat that has left", *assignee)
	}
	if got := leadSLANoticeRecipients(t, owner); len(got) != 0 {
		t.Errorf("notices addressed to %v, want none: nobody reads a seat that has left", got)
	}
}
