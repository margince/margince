// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What an inbound reply may publish.
//
// A contact kept to its owner records why, and a reply from the address ends
// exactly one of those reasons: mail we sent that nobody had answered. Each
// test narrows a contact through the writer that production uses, then lands a
// reply through the real capture sink. The first test is the case that must
// widen; every other one runs the same reply and must not.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAReplyPublishesAContactKeptOnlyBecauseNobodyHadAnswered(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "buyer@coldlead.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", "thr-cold"))
	requireNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)

	captureInboundThroughRealSink(t, e, e.Rep1, "reply-cold", email, "thr-cold")

	if visibility, _ := contactVisibility(t, e, email); visibility != "workspace" {
		t.Fatalf("after the address answered, the contact is %q, want workspace", visibility)
	}
	if reason := narrowingReasonOf(t, e, email); reason != nil {
		t.Errorf("a published contact still records narrowing reason %q", *reason)
	}
	// The mail-side readers ask the ledger. Left withheld, the contact would be
	// published while every message from it stayed held.
	if n := countIn(t, e, `SELECT count(*) FROM capture_pending_counterparty
		WHERE email = $1 AND withheld_from_workspace`, email); n != 0 {
		t.Errorf("%d ledger rows still withhold a sender whose contact was published", n)
	}
}

// The reply is asked the question the verdict asks before it narrows: an
// answer on a held thread may not publish its counterparty.
func TestAReplyOnAHeldThreadDoesNotPublish(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "buyer@heldreply.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", "thr-reply-held"))
	requireNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)
	seedThreadHold(t, e, "thr-reply-held", "held")

	captureInboundThroughRealSink(t, e, e.Rep1, "reply-held", email, "thr-reply-held")

	requireStillNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)
}

// Capture privacy is the importing seat's: a colleague's own answered mail is
// not authority to publish the contact this seat is keeping.
func TestAReplyInAnotherSeatsMailboxDoesNotPublish(t *testing.T) {
	e := setupWithOwnMailbox(t)
	e.WsExec(t, `
		INSERT INTO capture_connection
		       (user_id, provider, status, credential_ref, mail_posture, account_label)
		VALUES ($1, 'gmail', 'connected', 'vault:test', 'shared', 'b@authz.test')`, e.Rep2)
	const email = "buyer@twoseats.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", "thr-two-seats"))
	requireNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)

	captureInboundThroughRealSinkAs(mailboxOwnerCtx(e, e.Rep2), t, e, "b@authz.test",
		"reply-two-seats", email, "thr-two-seats")

	requireStillNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)
}

// The honest hard case: the hold is lifted before the reply arrives. Asking
// the thread again at reply time would find nothing and publish; the recorded
// reason is what still refuses.
func TestAReplyDoesNotPublishAContactAHoldNarrowedEvenOnceTheHoldIsLifted(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "counsel@heldmatter.example"
	activity := seedThreadedMail(t, e, email, "The matter", "inbound", "thr-hold")
	seedThreadHold(t, e, "thr-hold", "held")
	judge(t, e, email, capture.KindContact, activity)
	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)

	e.WsExec(t, `UPDATE capture_thread_verdict SET status = 'cleared' WHERE thread_key = 'thr-hold'`)
	replyOnFreshThread(t, e, email, "thr-hold-reply")

	requireStillNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)
}

// Mail we sent on a held thread is both outbound-unanswered and held. The hold
// is what gets recorded, or the reply would publish what the hold keeps quiet.
func TestOutboundMailOnAHeldThreadRecordsTheHold(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "partner@heldoutbound.example"
	activity := seedThreadedMail(t, e, email, "Terms", "outbound", "thr-held-out")
	seedThreadHold(t, e, "thr-held-out", "held")
	judge(t, e, email, capture.KindContact, activity)

	requireNarrowed(t, e, email, contacts.NarrowedConfidentialityHold)
}

func TestAReplyDoesNotPublishAnAdvisor(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "taxadvisor@ownbooks.example"
	judge(t, e, email, capture.KindAdvisor, seedThreadedMail(t, e, email, "Your return", "inbound", "thr-adv"))
	requireNarrowed(t, e, email, contacts.NarrowedAdvisor)

	replyOnFreshThread(t, e, email, "thr-adv-reply")

	requireStillNarrowed(t, e, email, contacts.NarrowedAdvisor)
}

func TestAReplyDoesNotPublishAContactAHumanMadePrivate(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "friend@ownchoice.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Hello", "inbound", "thr-human"))
	if visibility, _ := contactVisibility(t, e, email); visibility != "workspace" {
		t.Fatalf("the verdict left the contact %q, want workspace before the human narrows it", visibility)
	}
	private := "owner"
	if _, err := e.Contacts.UpdateContact(e.As(e.Rep1, nil, integration.RepPerms),
		contactIDFor(t, e, email), contacts.UpdateContactInput{Visibility: &private}); err != nil {
		t.Fatalf("making the contact private: %v", err)
	}
	requireNarrowed(t, e, email, contacts.NarrowedHumanDecided)

	replyOnFreshThread(t, e, email, "thr-human-reply")

	requireStillNarrowed(t, e, email, contacts.NarrowedHumanDecided)
}

// The sink usually mints the record before any verdict arrives, so the
// decision's reason has to land on a row that already exists.
func TestAVerdictRecordsItsReasonOnTheRecordTheSinkMintedFirst(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "notary@ownaffairs.example"
	seedAttestedOutbound(t, e, "sink-first-out", email, "thr-sink-first")
	captureInboundThroughRealSink(t, e, e.Rep1, "sink-first-in", email, "thr-sink-first")
	if reason := narrowingReasonOf(t, e, email); reason == nil || *reason != string(contacts.NarrowedAwaitingVerdict) {
		t.Fatalf("the sink recorded %v on a sender nothing has judged yet, want awaiting_verdict", reason)
	}
	dispositionID, queued := openDisposition(t, e, email)
	if !queued {
		t.Fatal("no verdict was opened for the sender the sink minted")
	}
	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindAdvisor}})
	requireNarrowed(t, e, email, contacts.NarrowedAdvisor)

	replyOnFreshThread(t, e, email, "thr-sink-first-reply")

	requireStillNarrowed(t, e, email, contacts.NarrowedAdvisor)
}

// A row narrowed before the reason existed records none. Absent is not
// outbound_no_answer, so it refuses rather than guessing.
func TestAReplyDoesNotPublishAContactThatRecordsNoReason(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "prospect@beforethecolumn.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", "thr-legacy"))
	requireNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)
	e.WsExec(t, `UPDATE contact SET narrowing_reason = NULL WHERE id = $1`, contactIDFor(t, e, email))

	captureInboundThroughRealSink(t, e, e.Rep1, "reply-legacy", email, "thr-legacy")

	if visibility, _ := contactVisibility(t, e, email); visibility != "owner" {
		t.Errorf("a reply published a contact that records no reason: visibility %q", visibility)
	}
}

// A reply only a colleague's mailbox caught is no answer to this owner, and it
// stays none when the owner's own next message arrives.
func TestAColleaguesReplyDoesNotPublishOnTheOwnersNextMessage(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "buyer@colleaguesreply.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", "thr-col"))
	captureInboundThroughRealSink(t, e, e.Rep2, "reply-col", email, "thr-col")

	captureInboundThroughRealSink(t, e, e.Rep1, "later-col", email, "thr-col-later")

	requireStillNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)
}

// A reply on a held thread stays no evidence when a later message from the
// same address lands on a thread nothing holds.
func TestAHeldReplyDoesNotPublishOnALaterUnheldMessage(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "buyer@heldthenopen.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", "thr-held-first"))
	seedThreadHold(t, e, "thr-held-first", "held")
	captureInboundThroughRealSink(t, e, e.Rep1, "reply-held-first", email, "thr-held-first")

	captureInboundThroughRealSink(t, e, e.Rep1, "later-open", email, "thr-open-later")

	requireStillNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)
}

// The live hook is not the only chance. A reply whose promotion failed is
// published by the verdict worker's pass.
func TestTheVerdictPassPublishesWhatTheLiveHookFailedToPublish(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "buyer@lostpromotion.example"
	loseTheLivePromotion(t, e, email, "thr-lost")

	publishAnswered(t, e)

	if visibility, _ := contactVisibility(t, e, email); visibility != "workspace" {
		t.Errorf("the verdict pass left an answered contact %q, want workspace", visibility)
	}
}

// One contact whose publication fails must not hold back the others on every
// tick; the pass still reports the failure.
func TestOneFailingContactDoesNotStopTheVerdictPass(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const stuck, fine = "buyer@stuckpromotion.example", "buyer@finepromotion.example"
	loseTheLivePromotion(t, e, stuck, "thr-stuck")
	loseTheLivePromotion(t, e, fine, "thr-fine")
	refuseUpdates(t, "contact", fmt.Sprintf("OLD.id = '%s'", contactIDFor(t, e, stuck)))

	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{}, CaptureConfig{}, slog.Default())
	if err := engine.PublishAnsweredContactsWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err == nil {
		t.Error("the pass swallowed the failed publication")
	}

	if visibility, _ := contactVisibility(t, e, fine); visibility != "workspace" {
		t.Errorf("a failing contact held back an answered one: visibility %q, want workspace", visibility)
	}
	requireNarrowed(t, e, stuck, contacts.NarrowedOutboundNoAnswer)
}

// loseTheLivePromotion captures the address's reply under the owner's grant
// without contact update, so the live promotion is refused by its own object
// gate and the contact is left owed publication.
func loseTheLivePromotion(t *testing.T, e *integration.Env, email, thread string) {
	t.Helper()
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", thread))
	ctx := mailboxOwnerCtx(e, e.Rep1)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"activity": {Create: true, Read: true},
		"contact":  {Create: true, Read: true},
		"company":  {Create: true, Read: true, Update: true},
	}
	captureInboundThroughRealSinkAs(principal.WithActor(ctx, actor), t, e, "a@authz.test", "reply-"+thread, email, thread)
	requireNarrowed(t, e, email, contacts.NarrowedOutboundNoAnswer)
}

// Upgrade state: a row narrowed before reasons existed. A later verdict judging
// the sender a contact must not publish it, since its reason may be a hold.
func TestALegacyNarrowingIsNotPublishedByALaterVerdict(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "counsel@legacyhold.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", "thr-legacy-v"))
	e.WsExec(t, `UPDATE contact SET narrowing_reason = NULL WHERE id = $1`, contactIDFor(t, e, email))
	// The same inbound verdict publishes an ordinary contact; see
	// TestAReplyDoesNotPublishAContactAHumanMadePrivate's first step.
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Re: matter", "inbound", "thr-legacy-v2"))

	if visibility, _ := contactVisibility(t, e, email); visibility != "owner" {
		t.Errorf("a later verdict published a legacy narrowed contact: visibility %q", visibility)
	}
}

// Upgrade state again: a later narrowing decision does not guess a legacy
// row's reason, so it cannot turn one into something a reply may publish.
func TestALegacyNarrowingKeepsItsUnknownReason(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const email = "counsel@legacyreason.example"
	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Intro", "outbound", "thr-legacy-r"))
	e.WsExec(t, `UPDATE contact SET narrowing_reason = NULL WHERE id = $1`, contactIDFor(t, e, email))

	judge(t, e, email, capture.KindContact, seedThreadedMail(t, e, email, "Again", "outbound", "thr-legacy-r2"))

	if reason := narrowingReasonOf(t, e, email); reason != nil {
		t.Errorf("a later verdict wrote %q over a legacy row's unknown reason", *reason)
	}
}

// setupWithOwnMailbox connects Rep1's mailbox under the shared posture. Its
// address is what makes a captured reply an import into Rep1's own mailbox,
// which is the evidence a reply publishes on; the shared posture opens no
// thread question that would read as a hold.
func setupWithOwnMailbox(t *testing.T) *integration.Env {
	t.Helper()
	e := integration.Setup(t)
	e.WsExec(t, `
		INSERT INTO capture_connection
		       (user_id, provider, status, credential_ref, mail_posture, account_label)
		VALUES ($1, 'gmail', 'connected', 'vault:test', 'shared', 'a@authz.test')`, e.Rep1)
	return e
}

// publishAnswered runs the verdict worker's reconciling pass.
func publishAnswered(t *testing.T, e *integration.Env) {
	t.Helper()
	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{}, CaptureConfig{}, slog.Default())
	if err := engine.PublishAnsweredContactsWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("publishing answered contacts: %v", err)
	}
}

// judge runs one verdict of the given kind over a ledger row for this address.
func judge(t *testing.T, e *integration.Env, email, kind string, activity ids.UUID) {
	t.Helper()
	id := seedPendingDisposition(t, e, email, email[strings.LastIndex(email, "@")+1:], activity)
	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{id.String(): kind}})
}

// replyOnFreshThread is the address answering mail we sent on a thread nothing
// holds, the same shape that publishes in the first test above.
func replyOnFreshThread(t *testing.T, e *integration.Env, email, thread string) {
	t.Helper()
	seedThreadedMail(t, e, email, "Following up", "outbound", thread)
	captureInboundThroughRealSink(t, e, e.Rep1, "reply-"+thread, email, thread)
}

func requireNarrowed(t *testing.T, e *integration.Env, email string, want contacts.NarrowingReason) {
	t.Helper()
	if visibility, found := contactVisibility(t, e, email); !found || visibility != "owner" {
		t.Fatalf("contact for %s: found=%v visibility=%q, want an owner-scoped record", email, found, visibility)
	}
	if got := narrowingReasonOf(t, e, email); got == nil || *got != string(want) {
		t.Fatalf("contact for %s records narrowing reason %v, want %q", email, got, want)
	}
}

// requireStillNarrowed runs the reconciling pass first, so a refusal holds on
// both paths that publish on an answer.
func requireStillNarrowed(t *testing.T, e *integration.Env, email string, want contacts.NarrowingReason) {
	t.Helper()
	publishAnswered(t, e)
	if visibility, _ := contactVisibility(t, e, email); visibility != "owner" {
		t.Errorf("a reply published a contact narrowed as %q: visibility %q", want, visibility)
	}
	if got := narrowingReasonOf(t, e, email); got == nil || *got != string(want) {
		t.Errorf("after the reply the contact records %v, want %q kept", got, want)
	}
}

func narrowingReasonOf(t *testing.T, e *integration.Env, email string) *string {
	t.Helper()
	var reason *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT p.narrowing_reason
			  FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
			 WHERE pe.email = $1 AND p.archived_at IS NULL`, email).Scan(&reason)
	}); err != nil {
		t.Fatalf("reading the narrowing reason of %s: %v", email, err)
	}
	return reason
}
