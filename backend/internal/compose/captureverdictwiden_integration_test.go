// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A `classified` mailbox holds each message until something judges its sender.
// These tests are about what happens when something does.
//
// Most of the file is refusals, for the same reason widenhistory's tests are:
// the release itself is one statement, and what makes it safe is everything it
// declines to publish — a sender judged real but not a person, a mailbox that
// asked to hold everything, a thread a confidentiality verdict already settled,
// and a message a second rule held for a reason of its own.

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The defect this whole change is about: a sender the classifier cleared, and
// mail that stays limited to its participants for good because nothing re-asks
// a question the ledger already answered.
func TestARealPersonVerdictReopensTheMailThePostureHeld(t *testing.T) {
	e := integration.Setup(t)
	const sender = "chi@fvhospital.example"
	mail := seedCapturedMail(t, e, sender, "FVH | Remaining specialities")
	seedPostureHeldImport(t, e, mail, e.Rep1)
	dispositionID := seedPendingDisposition(t, e, sender, "fvhospital.example", mail)

	if got, reason := audienceAndReason(t, e, mail); got != "participants" || reason != "posture" {
		t.Fatalf("the mail was born %q / %q, want participants / posture — the fixture is not held at all",
			got, reason)
	}

	judgeSenderAs(t, e, dispositionID, capture.KindPerson)

	if got, reason := audienceAndReason(t, e, mail); got != "workspace" || reason != "" {
		t.Errorf("after its sender was judged a real person the mail is %q / %q, want workspace: "+
			"a cleared verdict never re-opened the mail it cleared", got, reason)
	}
	if posture := importPosture(t, e, mail); posture != "shared" {
		t.Errorf("the import row still reads %q, want shared: the derivation will re-pin the row "+
			"to participants on the next recompute", posture)
	}
}

// status `real` is not the question. advisor, role_mailbox and
// company_sender all settle `real`, and none of them is a person whose
// mail a posture should stop holding — an advisor's most of all, since that
// record is deliberately kept to its owner.
func TestAVerdictThatIsRealButNotAPersonPublishesNothing(t *testing.T) {
	for _, kind := range []string{capture.KindAdvisor, capture.KindRoleMailbox, capture.KindCompanySender} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			sender := fmt.Sprintf("%s@kanzlei.example", kind)
			mail := seedCapturedMail(t, e, sender, "Aufhebungsvertrag")
			seedPostureHeldImport(t, e, mail, e.Rep1)
			dispositionID := seedPendingDisposition(t, e, sender, "kanzlei.example", mail)

			judgeSenderAs(t, e, dispositionID, kind)

			if got, _ := audienceAndReason(t, e, mail); got != "participants" {
				t.Errorf("a %s verdict published the mail as %q: the release asked for status "+
					"alone instead of asking whether a PERSON was behind the address", kind, got)
			}
		})
	}
}

// Noise settles the sender in the other direction entirely, and its mail is the
// mail the posture was right about.
func TestANoiseVerdictLeavesThePostureHeldMailHeld(t *testing.T) {
	e := integration.Setup(t)
	const sender = "newsletter@versand.example"
	mail := seedCapturedMail(t, e, sender, "Unser Newsletter")
	seedPostureHeldImport(t, e, mail, e.Rep1)
	dispositionID := seedPendingDisposition(t, e, sender, "versand.example", mail)

	judgeSenderAs(t, e, dispositionID, capture.KindNewsletter)

	if got, _ := audienceAndReason(t, e, mail); got != "participants" {
		t.Errorf("a newsletter verdict published its sender's mail as %q", got)
	}
}

// A `held` mailbox promises to hold whatever any classifier concludes. A
// verdict is an answer to `classified`'s question, not to this one.
func TestAClearedSenderNeverOpensAHeldMailbox(t *testing.T) {
	e := integration.Setup(t)
	const sender = "chi@fvhospital.example"
	mail := seedCapturedMail(t, e, sender, "FVH | Remaining specialities")
	seedImport(t, e, mail, e.Rep1, "held", nil, []string{"posture"})
	dispositionID := seedPendingDisposition(t, e, sender, "fvhospital.example", mail)

	judgeSenderAs(t, e, dispositionID, capture.KindPerson)

	if posture := importPosture(t, e, mail); posture != "held" {
		t.Errorf("the import row moved to %q: a mailbox that asked to hold everything had its "+
			"posture rewritten by a verdict about one sender", posture)
	}
}

// The posture was one of two reasons, so releasing on the posture's own answer
// would discard the other. The counterparty hold is the seat's separate
// decision about this correspondent and no verdict speaks for it.
func TestAClearedSenderDoesNotLiftACounterpartyHold(t *testing.T) {
	e := integration.Setup(t)
	const sender = "anwalt@studiolegal.example"
	mail := seedCapturedMail(t, e, sender, "Aufhebungsvertrag")
	seedImport(t, e, mail, e.Rep1, "classified", nil, []string{"counterparty", "posture"})
	dispositionID := seedPendingDisposition(t, e, sender, "studiolegal.example", mail)

	judgeSenderAs(t, e, dispositionID, capture.KindPerson)

	if posture := importPosture(t, e, mail); posture != "classified" {
		t.Errorf("the import row moved to %q: a message a seat's own counterparty hold also caught "+
			"was released by a verdict that knew nothing about that hold", posture)
	}
}

// A confidentiality verdict on the thread is written onto the import row
// without touching its reasons, so the exact-array match alone would not
// notice it. The status clause is what refuses.
func TestAThreadVerdictThatHeldOutranksAClearedSender(t *testing.T) {
	e := integration.Setup(t)
	const sender = "chi@fvhospital.example"
	mail := seedCapturedMail(t, e, sender, "Gehaltsanpassung")
	held := capture.VerdictHeld
	seedImport(t, e, mail, e.Rep1, "classified", &held, []string{"posture"})
	dispositionID := seedPendingDisposition(t, e, sender, "fvhospital.example", mail)

	judgeSenderAs(t, e, dispositionID, capture.KindPerson)

	if posture := importPosture(t, e, mail); posture != "classified" {
		t.Errorf("the import row moved to %q: a thread a confidentiality verdict held was "+
			"re-opened by a verdict about its sender", posture)
	}
}

// The import row's own status is not proof its thread is unjudged. Stamping a
// settled verdict onto a thread's siblings is bounded per transaction, so a
// long thread keeps rows reading NULL after a `held` verdict has committed —
// and those are precisely the messages a seat asked to keep back.
func TestAHeldThreadsUnstampedMessagesAreNotPublishedByASenderVerdict(t *testing.T) {
	e := integration.Setup(t)
	const sender = "chi@fvhospital.example"
	mail := seedCapturedMail(t, e, sender, "Gehaltsanpassung")
	seedPostureHeldImport(t, e, mail, e.Rep1)
	// The verdict is settled on the THREAD while this message's own import row
	// still says nothing — the state a bounded stamping pass leaves behind.
	holdThreadFor(t, e, mail, e.Rep1)
	dispositionID := seedPendingDisposition(t, e, sender, "fvhospital.example", mail)

	judgeSenderAs(t, e, dispositionID, capture.KindPerson)

	if got, _ := audienceAndReason(t, e, mail); got != "participants" {
		t.Errorf("a message on a thread its seat held is %q: the release read the import row's "+
			"own status as proof the thread was unjudged, and published mail a confidentiality "+
			"verdict had already held", got)
	}
}

// holdThreadFor settles this seat's confidentiality verdict on the message's
// thread, leaving the message's own import row unstamped.
func holdThreadFor(t *testing.T, e *integration.Env, activityID, seat ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		// The seeded mail carries no thread of its own, so the conversation this
		// verdict is about has to be named first.
		if _, err := tx.Exec(context.Background(),
			`UPDATE activity SET thread_key = 'thread-' || $1::text WHERE id = $1`,
			activityID); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, seen_addresses)
			SELECT a.thread_key, $2, 'held', ARRAY[]::text[]
			  FROM activity a WHERE a.id = $1`, activityID, seat)
		return err
	})
	if err != nil {
		t.Fatalf("holding the thread: %v", err)
	}
}

// A row written before the product recorded more than one reason says only
// which rule matched first. It is not PROVABLY posture-only, so it is refused
// rather than guessed at — the same trade widenhistory makes.
func TestMailCapturedBeforeReasonsWereRecordedStaysHeld(t *testing.T) {
	e := integration.Setup(t)
	const sender = "chi@fvhospital.example"
	mail := seedCapturedMail(t, e, sender, "FVH | Remaining specialities")
	seedImport(t, e, mail, e.Rep1, "classified", nil, nil)
	dispositionID := seedPendingDisposition(t, e, sender, "fvhospital.example", mail)

	judgeSenderAs(t, e, dispositionID, capture.KindPerson)

	if posture := importPosture(t, e, mail); posture != "classified" {
		t.Errorf("the import row moved to %q: a row that never recorded its reasons was released "+
			"as though the posture had been the only one", posture)
	}
}

// The reconciling pass reads the ledger directly rather than being told which
// verdict just landed, so the kind filter has to hold there on its own — this is
// the only place a non-person `real` verdict can reach the release at all.
func TestTheReconcilingPassLeavesARealButNonPersonSenderHeld(t *testing.T) {
	for _, kind := range []string{capture.KindAdvisor, capture.KindRoleMailbox, capture.KindCompanySender} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			sender := fmt.Sprintf("%s@kanzlei.example", kind)
			mail := seedCapturedMail(t, e, sender, "Aufhebungsvertrag")
			seedPostureHeldImport(t, e, mail, e.Rep1)
			dispositionID := seedPendingDisposition(t, e, sender, "kanzlei.example", mail)
			settleDisposition(t, e, dispositionID, capture.PendingStatusReal, kind)

			engine := NewCounterpartyVerdictEngine(e.Pool, nil, slog.Default())
			if err := engine.WidenClearedSendersWorkspace(
				principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
				t.Fatalf("the reconciling pass: %v", err)
			}

			if got, _ := audienceAndReason(t, e, mail); got != "participants" {
				t.Errorf("the pass published a %s sender's mail as %q: its scan asked for status "+
					"`real` alone, which every one of these kinds satisfies", kind, got)
			}
		})
	}
}

// The staging shape: a sender judged long before any release existed, whose
// mail nothing will ever revisit unless a pass goes looking. This is the half
// that makes the fix reach mail already captured.
func TestTheVerdictPassDrainsSendersClearedBeforeTheRelease(t *testing.T) {
	e := integration.Setup(t)
	const sender = "chi@fvhospital.example"
	mail := seedCapturedMail(t, e, sender, "FVH | Remaining specialities")
	seedPostureHeldImport(t, e, mail, e.Rep1)
	dispositionID := seedPendingDisposition(t, e, sender, "fvhospital.example", mail)
	settleDisposition(t, e, dispositionID, capture.PendingStatusReal, capture.KindPerson)

	engine := NewCounterpartyVerdictEngine(e.Pool, nil, slog.Default())
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := engine.WidenClearedSendersWorkspace(wsCtx); err != nil {
		t.Fatalf("the reconciling pass: %v", err)
	}

	if got, _ := audienceAndReason(t, e, mail); got != "workspace" {
		t.Fatalf("mail from a sender cleared before the release existed is still %q: the pass "+
			"that drains the backlog reached nothing", got)
	}
	// Idempotent: a second pass finds the same senders settled and nothing due,
	// so it must neither fail its own progress check nor move anything.
	if err := engine.WidenClearedSendersWorkspace(wsCtx); err != nil {
		t.Errorf("a second reconciling pass over drained senders: %v", err)
	}
}

// A sender with more held mail than one pass may spend has to finish on a later
// tick rather than holding a transaction open for all of it.
func TestTheReconcilingPassStopsAtItsBudgetAndFinishesOnTheNextTick(t *testing.T) {
	e := integration.Setup(t)
	const sender = "chi@fvhospital.example"
	const held = clearedSenderSweepBudget + 5
	var last ids.UUID
	for i := range held {
		last = seedCapturedMail(t, e, sender, fmt.Sprintf("held message %d", i))
		seedPostureHeldImport(t, e, last, e.Rep1)
	}
	dispositionID := seedPendingDisposition(t, e, sender, "fvhospital.example", last)
	settleDisposition(t, e, dispositionID, capture.PendingStatusReal, capture.KindPerson)

	engine := NewCounterpartyVerdictEngine(e.Pool, nil, slog.Default())
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := engine.WidenClearedSendersWorkspace(wsCtx); err != nil {
		t.Fatalf("the first reconciling pass: %v", err)
	}
	if left := stillHeld(t, e, sender); left == 0 {
		t.Fatalf("one pass released all %d messages: the budget never interrupted the drain, so a "+
			"large sender spends the whole fleet-wide run", held)
	}
	if err := engine.WidenClearedSendersWorkspace(wsCtx); err != nil {
		t.Fatalf("the second reconciling pass: %v", err)
	}
	if left := stillHeld(t, e, sender); left != 0 {
		t.Errorf("%d messages are still held after a second pass: the drain does not resume where "+
			"the budget stopped it", left)
	}
}

// judgeSenderAs drives the real engine to one scripted answer, so the release is
// reached the way production reaches it rather than by calling it directly.
func judgeSenderAs(t *testing.T, e *integration.Env, dispositionID ids.UUID, kind string) {
	t.Helper()
	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): kind}})
	if got := dispositionStatus(t, e, dispositionID); got == capture.PendingStatusPending {
		t.Fatalf("the disposition is still pending: the scripted answer never reached the ledger, " +
			"so this test asserts about a verdict that was never taken")
	}
}

// seedPostureHeldImport writes the import row a `classified` mailbox writes for
// a message whose sender it has not yet had judged.
func seedPostureHeldImport(t *testing.T, e *integration.Env, activityID, seat ids.UUID) {
	t.Helper()
	seedImport(t, e, activityID, seat, "classified", nil, []string{"posture"})
}

// seedImport writes one seat's capture_import row and the audience the birth
// decision would have written alongside it.
func seedImport(
	t *testing.T, e *integration.Env, activityID, seat ids.UUID,
	posture string, verdictStatus *string, reasons []string,
) {
	t.Helper()
	reason := ""
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import
			  (activity_id, user_id, posture_at_import, verdict_status, verdict_reason, verdict_reasons)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)`,
			activityID, seat, posture, verdictStatus, reason, reasons); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			UPDATE activity SET audience = 'participants', audience_reason = $2 WHERE id = $1`,
			activityID, reason)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the import row: %v", err)
	}
}

// settleDisposition records a verdict the way a pass on an earlier day would
// have left it: settled on the ledger, with no release behind it.
func settleDisposition(t *testing.T, e *integration.Env, id ids.UUID, status, kind string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET status = $2, kind = $3, resolved_at = now()
			 WHERE id = $1`, id, status, kind)
		return err
	})
	if err != nil {
		t.Fatalf("settling the disposition: %v", err)
	}
}

func audienceAndReason(t *testing.T, e *integration.Env, activityID ids.UUID) (audience, reason string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT audience, coalesce(audience_reason, '') FROM activity WHERE id = $1`,
			activityID).Scan(&audience, &reason)
	})
	if err != nil {
		t.Fatalf("reading the audience: %v", err)
	}
	return audience, reason
}

func importPosture(t *testing.T, e *integration.Env, activityID ids.UUID) string {
	t.Helper()
	var posture string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT posture_at_import FROM capture_import WHERE activity_id = $1`,
			activityID).Scan(&posture)
	})
	if err != nil {
		t.Fatalf("reading the import posture: %v", err)
	}
	return posture
}

// stillHeld counts what a further pass would still claim for this sender.
func stillHeld(t *testing.T, e *integration.Env, sender string) int {
	t.Helper()
	return countIn(t, e, `
		SELECT count(*) FROM capture_import i
		  JOIN activity a ON a.id = i.activity_id
		 WHERE a.counterparty_email = $1 AND i.posture_at_import = 'classified'`, sender)
}
