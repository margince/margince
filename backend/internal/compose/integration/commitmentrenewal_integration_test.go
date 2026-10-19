// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
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
				"kind": "commitment_made", "promiser_ref": "seat:" + e.Rep1.String(), "message_id": original.String(), "quote": originalQuote,
				"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
				"commitment": OpenCommitmentMeaning("2026-06-05", renewedQuote),
				"summary":    "Send enterprise pricing", "due_date": "2026-06-05", "confidence": 0.95,
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
			extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, original, "seat:"+e.Rep1.String(), quote, "2026-05-29")})
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
				"kind": "commitment_made", "promiser_ref": "seat:" + e.Rep1.String(), "message_id": original.String(), "quote": quote,
				"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
				"commitment": OpenCommitmentMeaning("2026-06-05", renewedQuote),
				"summary":    "Send pricing", "due_date": "2026-06-05", "confidence": 0.95,
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
	extractPass(t, e, &scriptedBrain{reply: commitmentReply(t, original, "contact:"+buyer.String(), quote, "")})
	if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1 AND NOT current_work`, original); got != 1 {
		t.Fatal("the original undated promise was not kept as history")
	}
	renewedQuote := "I will send those requirements by Friday."
	renewed := captureCommitmentMessage(t, e, buyer, "customer-renewal", "Re: Requirements", renewedQuote, "inbound", extractClock.Add(-24*time.Hour))
	party(t, renewed, "from", nil, &buyer)
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "promiser_ref": "contact:" + buyer.String(), "message_id": original.String(), "quote": quote,
		"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
		"commitment": OpenCommitmentMeaning("2026-06-05", renewedQuote),
		"summary":    "Send requirements", "due_date": "2026-06-05", "confidence": 0.95,
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
	opportunityQuote := "We need a new customer portal this quarter."
	opportunity := captureCommitmentMessage(t, e, buyer, "unverified-renewal", "Portal", opportunityQuote, "inbound", extractClock.Add(-12*time.Hour))
	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "promiser_ref": "seat:" + e.Rep1.String(), "message_id": original.String(), "quote": quote,
		"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
		"commitment": OpenCommitmentMeaning("2026-06-05", renewedQuote),
		"summary":    "Send pricing", "due_date": "2026-06-05", "confidence": 0.95,
	}, {
		"kind": "new_opportunity", "message_id": opportunity.String(), "quote": opportunityQuote,
		"summary": "Customer portal project", "confidence": 0.95,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	extractPass(t, e, &scriptedBrain{reply: string(reply)})
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
		t.Fatal("an unverified renewal created work")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='new_opportunity'`); got != 1 {
		t.Fatal("an unverifiable renewal discarded the independent opportunity")
	}
	brain := &scriptedBrain{reply: commitmentReply(t, original, "seat:"+e.Rep1.String(), quote, "")}
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
			"kind": "commitment_made", "promiser_ref": "seat:" + e.Rep1.String(), "message_id": original.String(), "quote": quote,
			"restated_message_id": message.String(), "restated_quote": body,
			"commitment": OpenCommitmentMeaning(due, body),
			"summary":    "Send pricing", "due_date": due, "confidence": 0.95,
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
 VALUES ($1, 'controller_pin', 'Test Controller', 'Statutory retention', $2)`, prior, extractClock)
	e.WsExec(t, `UPDATE activity SET retention_class='commercial_correspondence', retention_class_at=$2,
 archived_at=$2, restricted_at=$2, restricted_reason='statutory retention', restricted_until=$3 WHERE id=$1`, prior, extractClock, extractClock.AddDate(6, 0, 0))
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

func TestLegacySignalRefusalPreservesInvalidReadingsForRetry(t *testing.T) {
	for _, invalidMeaning := range []bool{false, true} {
		t.Run(map[bool]string{false: "unverified renewal", true: "invalid meaning"}[invalidMeaning], func(t *testing.T) {
			e := Setup(t)
			company := e.SeedCompany(t, "Acme", &e.Rep1)
			buyer := employeeOf(t, e, company, "Pat Buyer")
			quote := "I will send pricing."
			original := captureCommitmentMessage(t, e, buyer, "legacy-refused-renewal", "Pricing", quote, "outbound", extractClock.Add(-48*time.Hour))
			party(t, original, "from", &e.Rep1, nil)
			party(t, original, "to", nil, &buyer)
			renewedQuote := "I will send pricing tomorrow."
			renewed := captureCommitmentMessage(t, e, buyer, "legacy-refused-renewal", "Re: Pricing", renewedQuote, "outbound", extractClock.Add(-24*time.Hour))
			party(t, renewed, "from", &e.Rep2, nil)
			party(t, renewed, "to", nil, &buyer)
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				_, err := signals.RecordDerived(e.Admin(), tx, signals.DerivedSignal{
					Kind: "commitment_made", CompanyID: company, Summary: "Pricing promised.",
					Severity: "info", Fingerprint: "legacy-" + original.String(),
					Evidence: []signals.DerivedEvidence{{Snippet: quote, ActivityID: original}},
				}, extractClock.Add(-48*time.Hour))
				return err
			}); err != nil {
				t.Fatal(err)
			}
			meaning := OpenCommitmentMeaning("2026-06-05", renewedQuote)
			if invalidMeaning {
				meaning = nil
			}
			reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
				"kind": "commitment_made", "promiser_ref": "seat:" + e.Rep1.String(), "message_id": original.String(), "quote": quote,
				"restated_message_id": renewed.String(), "restated_quote": renewedQuote,
				"commitment": meaning,
				"summary":    "Send pricing", "due_date": "2026-06-05", "confidence": 0.95,
			}}})
			if err != nil {
				t.Fatal(err)
			}
			brain := &scriptedBrain{reply: string(reply)}
			if invalidMeaning {
				extractor := compose.NewSignalExtractor(e.Pool, brain, func() time.Time { return extractClock }, slog.Default())
				for range 2 {
					pass, err := extractor.RunWorkspace(signalScanCtx(e), ids.From[ids.WorkspaceKind](e.WS))
					if err == nil || !strings.Contains(err.Error(), "reading was refused") || pass.Converted != 0 {
						t.Fatalf("invalid legacy reading: converted=%d err=%v", pass.Converted, err)
					}
				}
				if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='commitment_made' AND status='open'`); got != 1 {
					t.Fatalf("retryable legacy signals=%d, want one", got)
				}
				if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
					t.Fatalf("invalid reading created %d tasks", got)
				}
				return
			}
			if pass := extractPassStats(t, e, brain); pass.Converted != 1 {
				t.Fatalf("converted %d legacy signals, want one", pass.Converted)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM signal WHERE kind='commitment_made' AND status='open'`); got != 0 {
				t.Fatal("the refused legacy renewal remained open")
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
				t.Fatal("an unverified renewal created work")
			}
			if pass := extractPassStats(t, e, brain); pass.Converted != 0 {
				t.Fatal("a settled legacy signal was converted again")
			}
		})
	}
}
