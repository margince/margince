// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestResolvingAPromiserPreservesTheExistingHumanDecision(t *testing.T) {
	for _, state := range []string{"pending", "rejected", "approved"} {
		t.Run(state, func(t *testing.T) {
			e := setupTranscript(t)
			e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
			first := e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.95)})
			if len(first.ProposalIDs) != 1 {
				t.Fatalf("unresolved promise created %d proposals", len(first.ProposalIDs))
			}
			if state != "pending" {
				reason := "Reviewed this promise"
				if _, err := e.svc.Decide(e.ctx, ids.From[ids.ApprovalKind](first.ProposalIDs[0]), state == "approved", &reason); err != nil {
					t.Fatal(err)
				}
			}
			before := e.taskCount(t)
			again := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.95)})
			if got := e.taskCount(t); got != before || len(again.ProposalIDs) != 0 {
				t.Fatalf("resolving attribution bypassed %s decision: tasks %d -> %d, new proposals %d", state, before, got, len(again.ProposalIDs))
			}
			if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind='commitment_task'`); got != 1 {
				t.Fatalf("same source clause acquired %d decisions", got)
			}
		})
	}
}
