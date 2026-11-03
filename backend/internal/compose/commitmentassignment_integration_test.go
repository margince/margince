// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCommitmentReadsMatchTheTaskWritersAssigneeRule(t *testing.T) {
	for _, state := range []string{"active", "invited", "deactivated", "archived", "agent"} {
		t.Run(state, func(t *testing.T) {
			e := setupTranscript(t)
			e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
			read := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.75)})
			if len(read.ProposalIDs) != 1 {
				t.Fatal("expected one proposal from the transcript")
			}
			id := ids.From[ids.ApprovalKind](read.ProposalIDs[0])
			switch state {
			case "archived":
				e.WsExec(t, `UPDATE app_user SET archived_at='2026-01-01T00:00:00Z' WHERE id=$1`, e.Rep2)
			case "agent":
				e.WsExec(t, `UPDATE app_user SET is_agent=true WHERE id=$1`, e.Rep2)
			default:
				e.WsExec(t, `UPDATE app_user SET status=$2 WHERE id=$1`, e.Rep2, state)
			}
			ctx := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
			got, err := e.svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			listed, _, err := e.svc.ListWire(ctx, approvals.ListInput{Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if got.TargetEntityType == nil || got.TargetEntityId == nil {
				t.Fatal("missing source target")
			}
			var targeted []crmcontracts.Approval
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				pending, err := e.svc.PendingForTarget(ctx, tx, *got.TargetEntityType, ids.UUID(*got.TargetEntityId))
				targeted = pending.Approvals
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if len(listed) != 1 || len(targeted) != 1 {
				t.Fatalf("list/target rows = %d/%d", len(listed), len(targeted))
			}
			// An invited colleague can hold work, as creating a task by hand allows.
			eligible := state == "active" || state == "invited"
			for _, approval := range []crmcontracts.Approval{got, listed[0], targeted[0]} {
				if approval.AssigneeInactive == nil || *approval.AssigneeInactive == eligible {
					t.Fatalf("%s unavailable = %v", state, approval.AssigneeInactive)
				}
				if approval.ProposedChange == nil || (*approval.ProposedChange)["seat_id"] != e.Rep2.String() {
					t.Fatal("reading changed the staged assignee")
				}
			}
			_, err = e.svc.Decide(ctx, id, true, nil)
			if eligible {
				if err != nil || e.taskCount(t) != 1 {
					t.Fatalf("eligible acceptance: tasks=%d error=%v", e.taskCount(t), err)
				}
			} else if err == nil || e.taskCount(t) != 0 {
				t.Fatalf("unavailable acceptance: tasks=%d error=%v", e.taskCount(t), err)
			}
		})
	}
}
