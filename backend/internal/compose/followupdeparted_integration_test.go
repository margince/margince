// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A deal whose owner has left is followed up by nobody. A card staged for a
// seat that no longer signs in is read by no one and staged again each night.

import "testing"

func TestADealWhoseOwnerHasLeftStagesNoFollowUpCard(t *testing.T) {
	cases := []struct {
		name, kind, update string
	}{
		{"deactivated owner, call evidence", "call", `status = 'deactivated'`},
		{"archived owner, call evidence", "call", `archived_at = now()`},
		// An answerable thread would be drafted for a live owner. For one who left
		// the pass falls back to the task proposal, and that is staged for nobody.
		{"deactivated owner, answerable email thread", "email", `status = 'deactivated'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupReconcile(t)
			deal := e.SeedDeal(t, "Left behind", e.pipeline, e.open, &e.Rep1)
			if tc.kind == "email" {
				e.seedAnswerableThread(t, deal, "Kickoff")
			} else {
				e.seedInteraction(t, deal, tc.kind, "Discovery call", 3)
			}
			e.WsExec(t, `UPDATE app_user SET `+tc.update+` WHERE id = $1`, e.Rep1)

			for night := 1; night <= 2; night++ {
				if err := e.reconcile(); err != nil {
					t.Fatalf("night %d: %v", night, err)
				}
			}

			if n := e.WsCount(t, `SELECT count(*) FROM approval
				WHERE target_entity_id = $1 AND kind IN ('deal_follow_up', 'held_draft')`, deal); n != 0 {
				t.Errorf("%d follow-up cards staged for a deal whose owner has left, want 0 — "+
					"nobody reads them, so they expire and are staged again", n)
			}
		})
	}
}
