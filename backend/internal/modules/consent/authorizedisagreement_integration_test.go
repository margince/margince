// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What the disagreement report says, and the one thing it must never get wrong:
// which direction a difference runs in.
//
// An operator reads this to decide whether enforcing the engine is safe. A
// report that called a stricter engine "more permissive" would tell them the
// opposite of the truth about mail that is going to stop.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// plantDecision writes one transmit decision with the two verdicts set.
func (e *resolveEnv) plantDecision(t *testing.T, category, reason, engine, legacy string) {
	t.Helper()
	delivery := e.plantDelivery(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   resolved_category, verdict, reason_code, legacy_verdict, mode, actor)
		VALUES ($1, 1, $2, 'someone@corp.test', 'transmit', $3, $4, $5, $6, 'observe', 'test')`,
		delivery, ids.NewV7(), category, engine, reason, legacy); err != nil {
		t.Fatalf("planting the decision: %v", err)
	}
}

// THE DIRECTION. An engine refusing what the old gate allowed is mail that
// STOPS when enforcement lands; the reverse is mail that starts. An operator
// deciding whether to flip is asking about the first.
//
// Mutation: swap the two sides of the EngineIsStricter comparison and this
// fails.
func TestTheReportSaysWhichWayADisagreementRuns(t *testing.T) {
	e := setupResolve(t)
	// The legacy transactional allow the engine will not reproduce.
	e.plantDecision(t, "account_notice", commsauthz.ReasonLegacyTransactionalUnevidenced,
		string(commsauthz.VerdictReview), string(commsauthz.VerdictAllow))
	// And the other way: a reply the engine allows on the thread alone, which
	// the old gate refused for want of a qualifying event.
	e.plantDecision(t, "reply_to_inbound", commsauthz.ReasonAllowed,
		string(commsauthz.VerdictAllow), string(commsauthz.VerdictDeny))

	report := e.report(t)
	if len(report) != 2 {
		t.Fatalf("the report carries %d shapes, want 2", len(report))
	}
	byCategory := map[string]Disagreement{}
	for _, d := range report {
		byCategory[d.Category] = d
	}
	if got := byCategory["account_notice"]; !got.EngineIsStricter {
		t.Error("an engine review against a legacy allow was not reported as stricter — that is mail that stops")
	}
	if got := byCategory["reply_to_inbound"]; got.EngineIsStricter {
		t.Error("an engine allow against a legacy deny was reported as stricter, which is backwards")
	}
}

// AGREEMENT IS NOT REPORTED. The report exists to show what differs, and rows
// where the two agreed are the overwhelming majority — including them would
// bury the answer in the thing that is already fine.
func TestTheReportCarriesOnlyDisagreements(t *testing.T) {
	e := setupResolve(t)
	e.plantDecision(t, "reply_to_inbound", commsauthz.ReasonAllowed,
		string(commsauthz.VerdictAllow), string(commsauthz.VerdictAllow))

	if report := e.report(t); len(report) != 0 {
		t.Fatalf("the report carries %d shapes for a decision both authorities agreed on, want none", len(report))
	}
}

// A DECISION THE OLD GATE NEVER ANSWERED is not a disagreement. legacy_verdict
// is nullable — a staging decision carries none — and NULL <> 'allow' is
// unknown in SQL, so this passes today; it is asserted so a rewrite that
// coalesced the column would fail rather than invent thousands of differences.
func TestADecisionWithNoLegacyAnswerIsNotADisagreement(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   resolved_category, verdict, reason_code, mode, actor)
		VALUES ($1, 0, $2, 'someone@corp.test', 'staging', 'marketing', 'review',
		        'no_compatible_evidence', 'observe', 'test')`,
		delivery, ids.NewV7()); err != nil {
		t.Fatal(err)
	}

	if report := e.report(t); len(report) != 0 {
		t.Fatalf("a decision the old gate never answered was reported as a disagreement: %+v", report)
	}
}

// ONE MESSAGE TO FIVE PEOPLE IS ONE MESSAGE. An operator asking what
// enforcement costs is asking how many SENDS stop, not how many rows changed —
// counting recipients would inflate the number by the size of the To line.
func TestOneMessageToSeveralRecipientsCountsOnce(t *testing.T) {
	e := setupResolve(t)
	delivery := e.plantDelivery(t)
	for _, address := range []string{"a@corp.test", "b@corp.test", "c@corp.test"} {
		if _, err := e.owner.Exec(context.Background(), `
			INSERT INTO communication_decision
			  (delivery_id, attempt, decision_set_id, recipient_address, phase,
			   resolved_category, verdict, reason_code, legacy_verdict, mode, actor)
			VALUES ($1, 1, $2, $3, 'transmit', 'account_notice', 'review',
			        'legacy_transactional_unevidenced', 'allow', 'observe', 'test')`,
			delivery, ids.NewV7(), address); err != nil {
			t.Fatal(err)
		}
	}

	report := e.report(t)
	if len(report) != 1 {
		t.Fatalf("the report carries %d shapes, want 1", len(report))
	}
	if report[0].Deliveries != 1 {
		t.Errorf("deliveries = %d, want 1 — three recipients are one message", report[0].Deliveries)
	}
	if report[0].Decisions != 3 {
		t.Errorf("decisions = %d, want 3 — the per-recipient count is the other half of the answer", report[0].Decisions)
	}
}

// The report discloses nothing about any subject, so it is gated on reading the
// installation's own settings rather than on a person. A caller without that
// grant is refused.
func TestTheReportNeedsTheSettingsReadGrant(t *testing.T) {
	e := setupResolve(t)
	e.dropGrant(t, authorizationModesObject)

	_, err := e.store.DisagreementReport(e.ctx)
	if err == nil {
		t.Fatal("a caller with no settings-read grant read the report")
	}
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("refused with %v, want ErrPermissionDenied", err)
	}
}

// report runs the reader.
func (e *resolveEnv) report(t *testing.T) []Disagreement {
	t.Helper()
	out, err := e.store.DisagreementReport(e.ctx)
	if err != nil {
		t.Fatalf("reading the report: %v", err)
	}
	return out
}

// THE SHAPE THE ROLLOUT DECISION TURNS ON. The legacy transactional purpose is
// an unconditional allow the engine will not reproduce without evidence, so
// every operational message sent under it is a disagreement — and enforcing
// would stop all of them.
//
// Asserted as a scenario rather than left to be discovered in production,
// because it is the single largest population the report will show and the one
// an operator has to understand before flipping anything.
func TestTheLegacyTransactionalAllowIsWhatEnforcementWouldStop(t *testing.T) {
	e := setupResolve(t)
	for range 3 {
		e.plantDecision(t, "account_notice", commsauthz.ReasonLegacyTransactionalUnevidenced,
			string(commsauthz.VerdictReview), string(commsauthz.VerdictAllow))
	}

	report := e.report(t)
	if len(report) != 1 {
		t.Fatalf("the report carries %d shapes, want the one", len(report))
	}
	got := report[0]
	if !got.EngineIsStricter {
		t.Fatal("the legacy transactional allow was not reported as mail that stops")
	}
	if got.Deliveries != 3 {
		t.Errorf("deliveries = %d, want 3", got.Deliveries)
	}
	if got.ReasonCode != commsauthz.ReasonLegacyTransactionalUnevidenced {
		t.Errorf("reason = %q, want the legacy-transactional code so an operator can see WHY", got.ReasonCode)
	}
}

// plantDecisionAt writes a transmit decision stamped at a chosen instant, which
// is what the window reads on.
func (e *resolveEnv) plantDecisionAt(t *testing.T, category, reason, engine, legacy string, at time.Time) {
	t.Helper()
	delivery := e.plantDelivery(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   resolved_category, verdict, reason_code, legacy_verdict, mode, actor, decided_at)
		VALUES ($1, 1, $2, 'someone@corp.test', 'transmit', $3, $4, $5, $6, 'observe', 'test', $7)`,
		delivery, ids.NewV7(), category, engine, reason, legacy, at); err != nil {
		t.Fatalf("planting the decision: %v", err)
	}
}

// reportSince reads the windowed report.
func (e *resolveEnv) reportSince(t *testing.T, since time.Time) []Disagreement {
	t.Helper()
	out, err := e.store.DisagreementReportSince(e.ctx, since)
	if err != nil {
		t.Fatalf("reading the windowed report: %v", err)
	}
	return out
}

// THE WINDOW IS WHAT MAKES A REPEATED READING SAY ANYTHING. Unbounded, each pass
// re-reads what the last one read plus a little, so a disagreement that appeared
// today is invisible under one that was fixed months ago.
//
// Mutation: drop the decided_at clause from the query and this fails, the old
// row coming back inside a window it is far outside.
func TestTheWindowExcludesADisagreementOlderThanIt(t *testing.T) {
	e := setupResolve(t)
	now := time.Now()
	e.plantDecisionAt(t, "account_notice", commsauthz.ReasonLegacyTransactionalUnevidenced,
		string(commsauthz.VerdictReview), string(commsauthz.VerdictAllow), now.Add(-30*24*time.Hour))
	e.plantDecisionAt(t, "reply_to_inbound", commsauthz.ReasonAllowed,
		string(commsauthz.VerdictAllow), string(commsauthz.VerdictDeny), now.Add(-time.Hour))

	within := e.reportSince(t, now.Add(-25*time.Hour))
	if len(within) != 1 {
		t.Fatalf("the windowed report carries %d shapes, want only the recent one", len(within))
	}
	if within[0].Category != "reply_to_inbound" {
		t.Errorf("the window returned %q, want the decision taken inside it", within[0].Category)
	}
}

// A ZERO WINDOW IS EVERY DECISION, which is what the one-off subcommand asks
// for. Both readings are one query text, and the zero time is what makes that
// possible: Go's zero time is year 1, so the bound admits every row.
//
// This pins the entry point rather than the clause — the test above holds the
// clause. Kept separate because the unwindowed reading is a caller with its own
// promise: DisagreementReport must keep answering about all of history however
// the windowed one is later tuned.
func TestAZeroWindowReadsEveryDecisionOnRecord(t *testing.T) {
	e := setupResolve(t)
	now := time.Now()
	e.plantDecisionAt(t, "account_notice", commsauthz.ReasonLegacyTransactionalUnevidenced,
		string(commsauthz.VerdictReview), string(commsauthz.VerdictAllow), now.Add(-90*24*time.Hour))

	if got := e.reportSince(t, time.Time{}); len(got) != 1 {
		t.Fatalf("the unwindowed report carries %d shapes, want the one on record", len(got))
	}
	// And the unwindowed entry point answers the same, because it IS this call.
	if got := e.report(t); len(got) != 1 {
		t.Fatalf("DisagreementReport carries %d shapes, want the same one", len(got))
	}
}

// THE WINDOW IS A LOWER BOUND, not a range. A decision taken after the pass
// started — one racing the read — is reported rather than dropped, because a
// disagreement nobody ever reports is the failure this whole reading exists to
// prevent.
func TestTheWindowHasNoUpperBound(t *testing.T) {
	e := setupResolve(t)
	e.plantDecisionAt(t, "account_notice", commsauthz.ReasonLegacyTransactionalUnevidenced,
		string(commsauthz.VerdictReview), string(commsauthz.VerdictAllow),
		time.Now().Add(time.Minute))

	if got := e.reportSince(t, time.Now().Add(-time.Hour)); len(got) != 1 {
		t.Fatalf("the report carries %d shapes, want the decision taken after the window opened", len(got))
	}
}
