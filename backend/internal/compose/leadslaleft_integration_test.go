// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// The event names the target the scan chose, and the run is delivered later. That
// is its first delivery, or an operator re-driving a failed run. The target can
// have left by then, and a refused write would lose the breach for good.
func TestAnEscalationWhoseTargetLeftStillWritesItsTask(t *testing.T) {
	cases := []struct {
		name, leave string
		intakeSeat  bool
	}{
		{"deactivated target, no intake seat", `status = 'deactivated'`, false},
		{"archived target, no intake seat", `archived_at = now()`, false},
		{"invited target, no intake seat", `status = 'invited'`, false},
		{"read-seat target, no intake seat", `seat_type = 'read'`, false},
		{"deactivated target, intake seat set", `status = 'deactivated'`, true},
		{"read-seat target, intake seat set", `seat_type = 'read'`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			owner := integration.OwnerConn(t)
			lead, ev, deadline, ctx := breachedLeadEvent(t, e, owner)
			if _, err := owner.Exec(context.Background(),
				`UPDATE app_user SET `+tc.leave+` WHERE id = $1`, e.Rep1); err != nil {
				t.Fatal(err)
			}
			if tc.intakeSeat {
				if _, err := owner.Exec(context.Background(),
					`INSERT INTO setting (key, value) VALUES ($1, to_jsonb($2::text))`,
					contacts.UnassignedEscalationUserID.Key(), e.Rep2.String()); err != nil {
					t.Fatal(err)
				}
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
			recipients := leadSLANoticeRecipients(t, owner)
			if !tc.intakeSeat {
				if assignee != nil || len(recipients) != 0 {
					t.Errorf("task assignee %v, notices to %v: want neither, as nobody reads a seat that has left",
						assignee, recipients)
				}
				return
			}
			if assignee == nil || *assignee != e.Rep2 {
				t.Errorf("task assignee = %v, want the intake seat %s", assignee, e.Rep2)
			}
			if len(recipients) != 1 || recipients[0] != e.Rep2 {
				t.Errorf("notices to %v, want only the intake seat %s", recipients, e.Rep2)
			}
		})
	}
}
