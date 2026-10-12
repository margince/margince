// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Redacting hidden noise mail destroys every copy of its text, not only the
// message row. The claims, the transcript reading, the staged proposal, the
// reply verdict and the stage evidence each hold the words or a conclusion
// drawn from them. Each is seeded here through the writer production uses.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// noiseQuote is the body seedMail writes. The claim writer refuses a quotation
// that is not in the message it cites.
const noiseQuote = "the message body"

func TestRedactingNoiseMailDestroysEveryCopyOfItsText(t *testing.T) {
	e := integration.Setup(t)
	noise := seedBulkCapturedMail(t, e, "deals@clearance.example", "spring clearance")
	dispositionID := seedPendingDisposition(t, e, "deals@clearance.example", "clearance.example", noise)

	// The same copies on a message nobody judged noise: the redaction is keyed
	// on the redacted message, never on matching text.
	kept := seedCapturedMail(t, e, "ops@partner.example", "delivery window")
	seedDerivedCopies(t, e, kept, noiseQuote)
	approvalID, evidenceID := seedDerivedCopies(t, e, noise, noiseQuote)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	ws := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := engine.RunWorkspace(ws, 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	backdateArchive(t, e, noise)
	if err := engine.RedactNoiseWorkspace(ws, capture.NoiseUndoWindow, 0); err != nil {
		t.Fatalf("redaction sweep: %v", err)
	}

	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NULL`, noise); n != 1 {
		t.Fatal("the sweep did not redact the noise message, so the rest of this test proves nothing")
	}
	for _, derived := range derivedCopyQueries {
		if n := countIn(t, e, derived.query, noise); n != 0 {
			t.Errorf("%s still holds %d row(s) derived from the redacted noise message", derived.table, n)
		}
		if n := countIn(t, e, derived.query, kept); n != 1 {
			t.Errorf("%s lost the copy of a message the sweep did not redact (%d rows, want 1)", derived.table, n)
		}
	}
	if !snippetCleared(t, e, evidenceID) {
		t.Error("the stage evidence still quotes the redacted noise message")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND decision_reason = $2`,
		approvalID, privacy.NoiseSourceWithdrawal); n != 1 {
		t.Error("the proposal quoting the message was not withdrawn with the reason that names a noise redaction")
	}
}

// derivedCopyQueries counts, per derived table, the rows read from one
// activity. Each holds the words of the message or a conclusion drawn from
// them.
var derivedCopyQueries = []struct{ table, query string }{
	{"conversation_claim", `SELECT count(*) FROM conversation_claim WHERE source_activity_id = $1`},
	{"transcript_read", `SELECT count(*) FROM transcript_read WHERE activity_id = $1`},
	{"activity_reply_verdict_history", `SELECT count(*) FROM activity_reply_verdict_history WHERE activity_id = $1`},
	{"activity.reply_verdict", `SELECT count(*) FROM activity WHERE id = $1 AND reply_verdict IS NOT NULL`},
	{"approval", `SELECT count(*) FROM approval
		WHERE coalesce(evidence::text, '') LIKE '%' || $1::text || '%'
		  AND coalesce(evidence::text, '') || coalesce(proposed_change::text, '') || coalesce(summary, '')
		      LIKE '%` + noiseQuote + `%'`},
	{"deal_stage_evidence.snippet", `SELECT count(*) FROM deal_stage_evidence
		WHERE source_id = $1 AND snippet IS NOT NULL`},
}

// seedDerivedCopies writes one copy of each kind onto a captured message,
// through the writers production uses, while the message is still visible.
func seedDerivedCopies(t *testing.T, e *integration.Env, activityID ids.UUID, quote string) (ids.ApprovalID, ids.UUID) {
	t.Helper()
	contact := e.SeedContact(t, "Clearance Desk "+activityID.String()[:8], nil)
	if _, err := e.Contacts.RecordConversationClaim(e.Admin(), contacts.ClaimInput{
		ContactID:  ids.From[ids.ContactKind](contact),
		Kind:       "commitment_theirs",
		Body:       "Send the clearance list",
		ActivityID: activityID,
		Quote:      quote,
		Source:     "manual",
	}); err != nil {
		t.Fatalf("recording the claim: %v", err)
	}
	e.WsExec(t, `
		INSERT INTO transcript_read (id, activity_id, status, line_count, requested_by, started_at, finished_at)
		VALUES ($1, $2, 'done', 1, 'human:seed', now(), now())`, ids.NewV7(), activityID)
	applied, err := activities.NewStore(e.DB()).SetReplyVerdict(e.Admin(), activityID,
		activities.ReplyVerdictNegative, "reply-classifier-test")
	if err != nil || !applied {
		t.Fatalf("setting the reply verdict: applied=%v err=%v", applied, err)
	}
	return stageQuotingProposal(t, e, activityID, quote, "Unsubscribe from the clearance list"),
		evidenceQuoting(t, e, activityID, quote)
}

// The other two arms that empty a message clear its reply verdict too. A
// verdict is a conclusion read from the words, so it does not outlive them.
func TestTheRetentionSweepClearsTheReplyVerdictOfAMessageItEmpties(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action)
		VALUES ('activity', NULL, 100, 'erase')`)
	agedOut := seedCapturedMail(t, e, "buyer@aged.example", "old reply")
	if applied, err := activities.NewStore(e.DB()).SetReplyVerdict(e.Admin(), agedOut,
		activities.ReplyVerdictPositive, "reply-classifier-test"); err != nil || !applied {
		t.Fatalf("setting the reply verdict: applied=%v err=%v", applied, err)
	}
	e.WsExec(t, `UPDATE activity SET occurred_at = now() - interval '400 days' WHERE id = $1`, agedOut)

	svc := NewRetentionServiceFor(e.DB(), nil, slog.Default())
	integration.SettleIntoInstall(t, e)
	if err := svc.EvaluateInstallation(integration.RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("EvaluateInstallation → %v", err)
	}
	assertEmptiedWithoutVerdict(t, e, agedOut)
}

func TestAControllerReleaseClearsTheReplyVerdictOfTheMessageItEmpties(t *testing.T) {
	e := integration.Setup(t)
	// Seeded held, with the verdict it was given before the hold. The verdict
	// writer refuses a restricted row. The restriction is pinned on insert for
	// the reason TestAControllerReleasingARestrictionSaysSoOnTheCardsItWithdrew gives.
	held := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, subject, body, direction, counterparty_email, occurred_at,
		                      source, source_system, source_id, captured_by,
		                      retention_class, retention_class_at,
		                      restricted_at, restricted_until, restricted_reason, archived_at,
		                      reply_verdict, reply_verdict_at, reply_verdict_by)
		VALUES ($1, 'email', 'Re: order', 'Yes, go ahead.', 'inbound', 'buyer@held.example',
		        now() - interval '30 days', 'capture_email', 'imap', $2, 'human:seed',
		        'commercial_correspondence', now(), now(), now() + interval '6 years',
		        'commercial_correspondence', now(), 'positive', now(), 'reply-classifier-test')`,
		held, "held-"+held.String())

	reason, err := privacy.ParseStatedReason("the supplier obligation ended: contract closed out")
	if err != nil {
		t.Fatal(err)
	}
	eraser := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB()))
	if err := eraser.ReleaseRestriction(releaseControllerCtx(e), held, reason); err != nil {
		t.Fatalf("releasing → %v", err)
	}
	assertEmptiedWithoutVerdict(t, e, held)
}

func assertEmptiedWithoutVerdict(t *testing.T, e *integration.Env, id ids.UUID) {
	t.Helper()
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NULL`, id); n != 1 {
		t.Fatal("the message was not emptied, so this test proves nothing about its verdict")
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND reply_verdict IS NOT NULL`, id); n != 0 {
		t.Error("the message was emptied and kept the reply verdict read from it")
	}
}
