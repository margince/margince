// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestMailReadingKeepsValidNonActionsAndRetriesInvalidMeaning(t *testing.T) {
	for _, tc := range []struct {
		name, words, state, condition, precision, deadline string
		tasks, calls                                       int
	}{
		{"open range", "I will send pricing next week.", "open", "", "range", "next week", 1, 1},
		{"completed", "I sent pricing this morning.", "completed", "", "none", "", 0, 1},
		{"conditional", "I will send pricing if the buyer agrees.", "conditional", "if the buyer agrees", "none", "", 0, 1},
		{"optional", "I could send pricing if useful.", "optional", "", "none", "", 0, 1},
		{"request", "Please send pricing.", "request", "", "none", "", 0, 1},
		{"empty result", "Thank you.", "empty", "", "none", "", 0, 1},
		{"missing meaning", "I will send pricing.", "", "", "none", "", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			company := e.SeedCompany(t, "Example buyer", &e.Rep1)
			buyer := employeeOf(t, e, company, "Example buyer contact")
			message := captureCommitmentMessage(t, e, buyer, "pricing-meaning", "Pricing", tc.words, "outbound", extractClock.Add(-48*time.Hour))
			party(t, message, "from", &e.Rep1, nil)
			party(t, message, "to", nil, &buyer)
			var meaning *crmcontracts.CommitmentMeaning
			if tc.state != "" {
				meaning = &crmcontracts.CommitmentMeaning{State: tc.state, Condition: tc.condition, DeadlinePrecision: tc.precision, DeadlineQuote: tc.deadline}
			}
			events := []map[string]any{{"kind": "commitment_made", "message_id": message.String(), "promiser_ref": "seat:" + e.Rep1.String(), "summary": "Send pricing", "quote": tc.words, "due_date": "", "confidence": 0.99, "commitment": meaning}}
			if tc.state == "empty" {
				events = []map[string]any{}
			}
			answer, err := json.Marshal(map[string]any{"events": events})
			if err != nil {
				t.Fatal(err)
			}
			brain := &scriptedBrain{reply: string(answer)}
			extractPass(t, e, brain)
			extractPass(t, e, brain)
			if brain.calls != tc.calls {
				t.Fatalf("model calls = %d, want %d", brain.calls, tc.calls)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != tc.tasks {
				t.Fatalf("tasks = %d, want %d", got, tc.tasks)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND due_at IS NOT NULL`); got != 0 {
				t.Fatal("reading invented a due day")
			}
			if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind='commitment_task'`); got != 0 {
				t.Fatalf("unexpected proposals: %d", got)
			}
		})
	}
}

func TestInvalidCommitmentDoesNotHideAValidRiskOrAdvanceTheReading(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Example buyer", &e.Rep1)
	buyer := employeeOf(t, e, company, "Example buyer contact")
	words := "We are ending the contract. I will send pricing."
	message := captureCommitmentMessage(t, e, buyer, "risk-and-invalid-promise", "Contract", words, "inbound", extractClock.Add(-48*time.Hour))
	party(t, message, "from", nil, &buyer)
	events := []map[string]any{
		{"kind": "contract_ended", "message_id": message.String(), "summary": "Contract ended", "quote": "We are ending the contract.", "due_date": "", "confidence": 0.99},
		{"kind": "commitment_made", "message_id": message.String(), "promiser_ref": "contact:" + buyer.String(), "summary": "Send pricing", "quote": "I will send pricing.", "due_date": "", "confidence": 0.99},
	}
	answer, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		t.Fatal(err)
	}
	brain := &scriptedBrain{reply: string(answer)}
	for range 2 {
		extractPass(t, e, brain)
	}
	if brain.calls != 2 {
		t.Fatalf("invalid reading was not retried: %d calls", brain.calls)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='contract_ended'`); got != 1 {
		t.Fatalf("risk signals=%d, want one", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim`); got != 0 {
		t.Fatalf("invalid commitment created %d claims", got)
	}
}
