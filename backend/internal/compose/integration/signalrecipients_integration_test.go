// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCapturedSharedPromiseKeepsOnlyItsNamedRecipients(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := employeeOf(t, e, company, "Ines Buyer")
	morgan := employeeOf(t, e, company, "Morgan Buyer")
	copied := employeeOf(t, e, company, "Pat Observer")
	quote := "Ines and Morgan, I will send both of you the revised pricing by June 5, 2026."
	message := captureCommitmentMessage(t, e, ines, "shared-pricing", "Pricing", quote, "outbound", extractClock.Add(-48*time.Hour), commitmentMailRecipient{morgan, "to"}, commitmentMailRecipient{copied, "cc"})
	party(t, message, "from", &e.Rep1, nil)
	party(t, message, "to", nil, &ines)
	response, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": message.String(), "promiser_ref": "seat:" + e.Rep1.String(),
		"recipient_refs": []string{"contact:" + ines.String(), "contact:" + morgan.String()},
		"summary":        "Send the revised pricing", "quote": quote, "due_date": "2026-06-05", "confidence": 0.95,
		"commitment": OpenCommitmentMeaning("2026-06-05", quote),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	brain := &scriptedBrain{reply: string(response)}
	for range 2 {
		extractPass(t, e, brain)
		if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND source_activity_id=$1 AND assignee_id=$2`, message, e.Rep1); got != 1 {
			t.Fatalf("shared promise produced %d tasks, want one owned by its promiser", got)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim c JOIN activity a ON a.id=c.task_activity_id WHERE c.source_activity_id=$1 AND c.contact_id IN ($2,$3) AND c.status='open' AND a.kind='task'`, message, ines, morgan); got != 2 {
			t.Fatalf("shared promise linked %d recipient claims, want both named buyers", got)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1 AND contact_id=$2`, message, copied); got != 0 {
			t.Fatalf("copied observer acquired %d obligations", got)
		}
		e.WsExec(t, `DELETE FROM signal_thread_scan`)
	}
}
