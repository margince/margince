// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARenewedOldPromiseUsesItsCurrentDeadlineWithoutDuplicatingWork(t *testing.T) {
	for _, historyGap := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete thread", true: "history gap"}[historyGap], func(t *testing.T) {
			e := Setup(t)
			company := e.SeedCompany(t, "Acme", &e.Rep1)
			buyer := employeeOf(t, e, company, "Pat Buyer")
			originalQuote := "I will send the enterprise pricing."
			originalAt := extractClock.AddDate(0, 0, -45)
			original := captureCommitmentMessage(t, e, buyer, "renewed-pricing", "Pricing", originalQuote, "outbound", originalAt)
			party(t, original, "from", &e.Rep1, nil)
			party(t, original, "to", nil, &buyer)
			if historyGap {
				for i := range 28 {
					captureCommitmentMessage(t, e, buyer, "renewed-pricing", "Re: Pricing", "Status update.", "inbound", originalAt.Add(time.Duration(i+1)*time.Hour))
				}
				if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
					_, err := signals.RecordDerived(e.Admin(), tx, signals.DerivedSignal{
						Kind: "commitment_made", CompanyID: company, Summary: "Send pricing", Severity: "info", Fingerprint: "renewal-" + original.String(),
						Evidence: []signals.DerivedEvidence{{Snippet: originalQuote, ActivityID: original}},
					}, originalAt)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			renewedQuote := "I will send that enterprise pricing by Friday."
			renewed := captureCommitmentMessage(t, e, buyer, "renewed-pricing", "Re: Pricing", renewedQuote, "outbound", extractClock.Add(-24*time.Hour))
			party(t, renewed, "from", &e.Rep1, nil)
			party(t, renewed, "to", nil, &buyer)
			reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
				"kind": "commitment_made", "message_id": original.String(), "quote": originalQuote,
				"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
				"summary": "Send enterprise pricing", "due_date": "2026-06-05", "confidence": 0.95,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			brain := &scriptedBrain{reply: string(reply)}
			extractPass(t, e, brain)
			extractPass(t, e, brain)
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND NOT is_done AND archived_at IS NULL`); got != 1 {
				t.Fatalf("renewal left %d open tasks", got)
			}
			if got := e.WsScalar(t, `SELECT to_char(due_at,'YYYY-MM-DD') FROM activity WHERE kind='task'`); got != "2026-06-05" {
				t.Fatalf("renewed deadline = %q", got)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1 AND current_work AND $2=ANY(evidence_ids)`, original, renewed); got != 1 {
				t.Fatal("the current commitment does not name its renewal evidence")
			}
		})
	}
}

func TestRenewalUpdatesAutomaticWorkAndKeepsHumanWork(t *testing.T) {
	for _, state := range []string{"automatic", "edited", "completed", "archived"} {
		t.Run(state, func(t *testing.T) {
			e := Setup(t)
			company := e.SeedCompany(t, "Acme", &e.Rep1)
			buyer := employeeOf(t, e, company, "Pat Buyer")
			quote := "I will send pricing by Friday."
			original := captureCommitmentMessage(t, e, buyer, "renew-existing", "Pricing", quote, "outbound", extractClock.AddDate(0, 0, -5))
			party(t, original, "from", &e.Rep1, nil)
			party(t, original, "to", nil, &buyer)
			extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, original, quote, "2026-05-29")})
			taskID := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM activity WHERE kind='task'`))
			store := activities.NewStore(e.DB())
			if state == "edited" || state == "completed" {
				patch := activities.UpdateActivityInput{}
				if state == "edited" {
					patch.Subject = new("My negotiated pricing task")
				} else {
					patch.IsDone = new(true)
				}
				if _, err := store.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](taskID), patch); err != nil {
					t.Fatal(err)
				}
			}
			if state == "archived" {
				if _, err := store.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](taskID), nil); err != nil {
					t.Fatal(err)
				}
			}
			renewedQuote := "I will send that pricing next Friday instead."
			renewed := captureCommitmentMessage(t, e, buyer, "renew-existing", "Re: Pricing", renewedQuote, "outbound", extractClock.Add(-24*time.Hour))
			party(t, renewed, "from", &e.Rep1, nil)
			party(t, renewed, "to", nil, &buyer)
			reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
				"kind": "commitment_made", "message_id": original.String(), "quote": quote,
				"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
				"summary": "Send pricing", "due_date": "2026-06-05", "confidence": 0.95,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			extractPass(t, e, &scriptedBrain{reply: string(reply)})
			extractPass(t, e, &scriptedBrain{reply: string(reply)})
			want := "2026-05-29"
			if state == "automatic" {
				want = "2026-06-05"
			}
			if got := e.WsScalar(t, `SELECT to_char(due_at,'YYYY-MM-DD') FROM activity WHERE id=$1`, taskID); got != want {
				t.Fatalf("deadline = %s, want %s", got, want)
			}
			if got := e.WsScalar(t, `SELECT to_char(due_at,'YYYY-MM-DD') FROM conversation_claim WHERE source_activity_id=$1`, original); got != want {
				t.Fatalf("claim deadline = %s, want %s", got, want)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 1 {
				t.Fatalf("renewal left %d tasks", got)
			}
			if state == "edited" && e.WsScalar(t, `SELECT subject FROM activity WHERE id=$1`, taskID) != "My negotiated pricing task" {
				t.Fatal("renewal replaced the human's wording")
			}
			if state == "completed" && e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND is_done`, taskID) != 1 {
				t.Fatal("renewal reopened completed work")
			}
			if state == "archived" && e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND archived_at IS NOT NULL`, taskID) != 1 {
				t.Fatal("renewal restored archived work")
			}
		})
	}
}

func TestCustomerRenewalUpdatesTheExistingHistoricalClaimDeadline(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	buyer := employeeOf(t, e, company, "Pat Buyer")
	quote := "I will send our requirements."
	original := captureCommitmentMessage(t, e, buyer, "customer-renewal", "Requirements", quote, "inbound", extractClock.AddDate(0, 0, -45))
	party(t, original, "from", nil, &buyer)
	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, original, quote, "")})
	if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1 AND NOT current_work`, original); got != 1 {
		t.Fatal("the original undated promise was not kept as history")
	}
	renewedQuote := "I will send those requirements by Friday."
	renewed := captureCommitmentMessage(t, e, buyer, "customer-renewal", "Re: Requirements", renewedQuote, "inbound", extractClock.Add(-24*time.Hour))
	party(t, renewed, "from", nil, &buyer)
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": original.String(), "quote": quote,
		"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
		"summary": "Send requirements", "due_date": "2026-06-05", "confidence": 0.95,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	extractPass(t, e, &scriptedBrain{reply: string(reply)})
	if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE source_activity_id=$1`, original); got != 1 {
		t.Fatalf("renewal left %d claims", got)
	}
	if got := e.WsScalar(t, `SELECT to_char(due_at,'YYYY-MM-DD') FROM conversation_claim WHERE source_activity_id=$1`, original); got != "2026-06-05" {
		t.Fatalf("renewed customer deadline = %s", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1 AND current_work AND renewed_by=$2`, original, renewed); got != 1 {
		t.Fatal("the renewed claim is not current work with explicit renewal evidence")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
		t.Fatal("a customer's promise became our task")
	}
}

func TestUnverifiedRenewalRemainsDueForAnotherReading(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	buyer := employeeOf(t, e, company, "Pat Buyer")
	quote := "I will send pricing."
	original := captureCommitmentMessage(t, e, buyer, "unverified-renewal", "Pricing", quote, "outbound", extractClock.Add(-48*time.Hour))
	party(t, original, "from", &e.Rep1, nil)
	party(t, original, "to", nil, &buyer)
	renewedQuote := "I will send pricing tomorrow."
	renewed := captureCommitmentMessage(t, e, buyer, "unverified-renewal", "Re: Pricing", renewedQuote, "outbound", extractClock.Add(-24*time.Hour))
	party(t, renewed, "from", &e.Rep2, nil)
	party(t, renewed, "to", nil, &buyer)
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": original.String(), "quote": quote,
		"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
		"summary": "Send pricing", "due_date": "2026-06-05", "confidence": 0.95,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	extractPass(t, e, &scriptedBrain{reply: string(reply)})
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
		t.Fatal("an unverified renewal created work")
	}
	brain := &scriptedBrain{reply: commitmentReply(t, original, quote, "")}
	extractPass(t, e, brain)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND source_activity_id=$1`, original); got != 1 {
		t.Fatal("rejecting a renewal permanently lost the original promise")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task' AND due_at IS NOT NULL`); got != 0 {
		t.Fatal("the original inherited the rejected renewal's deadline")
	}
}

func TestRefusedClaimRenewalRollsBackTheTaskAndItsTrace(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	buyer := employeeOf(t, e, company, "Pat Buyer")
	quote := "I will send pricing."
	original := captureCommitmentMessage(t, e, buyer, "rollback-renewal", "Pricing", quote, "outbound", extractClock.AddDate(0, 0, -5))
	party(t, original, "from", &e.Rep1, nil)
	party(t, original, "to", nil, &buyer)
	renew := func(body, due string, at time.Time) ids.UUID {
		t.Helper()
		message := captureCommitmentMessage(t, e, buyer, "rollback-renewal", "Re: Pricing", body, "outbound", at)
		party(t, message, "from", &e.Rep1, nil)
		party(t, message, "to", nil, &buyer)
		reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
			"kind": "commitment_made", "message_id": original.String(), "quote": quote,
			"restated_message_id": message.String(), "restated_quote": body,
			"summary": "Send pricing", "due_date": due, "confidence": 0.95,
		}}})
		if err != nil {
			t.Fatal(err)
		}
		extractPass(t, e, &scriptedBrain{reply: string(reply)})
		return message
	}
	prior := renew("I will send pricing by Friday.", "2026-06-05", extractClock.Add(-48*time.Hour))
	task := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM activity WHERE kind='task'`))
	before := e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, task)
	admission := e.WsScalar(t, `SELECT to_jsonb(d)::text FROM activity_commitment_admission d WHERE source_activity_id=$1`, original)
	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, task)
	e.WsExec(t, `INSERT INTO activity_retention_evidence (activity_id, basis, decided_by_name, reason, qualified_at)
 VALUES ($1, 'controller_pin', 'Test Controller', 'Statutory retention', now())`, prior)
	e.WsExec(t, `UPDATE activity SET retention_class='commercial_correspondence', retention_class_at=now(),
 archived_at=now(), restricted_at=now(), restricted_reason='statutory retention', restricted_until=now()+interval '6 years' WHERE id=$1`, prior)
	renew("I will send pricing next Friday instead.", "2026-06-12", extractClock.Add(-24*time.Hour))
	if after := e.WsScalar(t, `SELECT to_jsonb(a)::text FROM activity a WHERE id=$1`, task); after != before {
		t.Fatal("a refused claim renewal committed the task update")
	}
	if after := e.WsScalar(t, `SELECT to_jsonb(d)::text FROM activity_commitment_admission d WHERE source_activity_id=$1`, original); after != admission {
		t.Fatal("a refused claim renewal changed its admission evidence")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, task); got != audits {
		t.Fatal("a refused claim renewal retained its task audit")
	}
	if got := e.WsScalar(t, `SELECT to_char(due_at,'YYYY-MM-DD') FROM conversation_claim WHERE source_activity_id=$1`, original); got != "2026-06-05" {
		t.Fatalf("claim deadline changed to %s", got)
	}
}
