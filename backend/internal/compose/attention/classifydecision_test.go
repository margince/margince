// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import "testing"

func TestCommercialStageDecisionsAreNotRoutineCleanup(t *testing.T) {
	for _, kind := range []string{"advance_deal", "progress_deal", "stage_progression"} {
		t.Run(kind, func(t *testing.T) {
			row := classifyDecision(item("proposal", "approval", withKind(kind)), rankInstant).item
			if row.Level != levelAgreed || row.Consequence != "none" || len(row.Because) != 0 {
				t.Fatalf("commercial decision acquired a hygiene claim: %+v", row)
			}
		})
	}
	send := classifyDecision(item("send", "approval", withKind("send_email")), rankInstant).item
	if send.Level != levelBlocking || send.Consequence != "work_blocked" || len(send.Because) != 1 || send.Because[0].Kind != "blocks_customer_work" {
		t.Fatalf("send lost its blocking classification: %+v", send)
	}
	cleanup := classifyDecision(item("merge", "approval", withKind("merge_contacts")), rankInstant).item
	if cleanup.Level != levelRoutine || cleanup.Consequence != "data_drifts" || len(cleanup.Because) != 1 || cleanup.Because[0].Kind != "routine" {
		t.Fatalf("contact cleanup lost its routine classification: %+v", cleanup)
	}
}
