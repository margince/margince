// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The owner's waiting list says what each address waits on, and an owner's
// answer about a sender reaches the verdict pass however the question stood.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheWaitingListSaysWhatEachAddressWaitsOn(t *testing.T) {
	e := integration.Setup(t)
	want := map[string]capture.AwaitingState{
		"decided@state.test":     capture.AwaitingYourAnswerApplying,
		"classifying@state.test": capture.AwaitingClassifier,
		"unsure@state.test":      capture.AwaitingUnsure,
		"queued@state.test":      capture.AwaitingQueued,
		"nothing@state.test":     capture.AwaitingNothingToJudge,
	}
	contacts := map[string]ids.UUID{}
	for email := range want {
		contacts[email] = seedOwnerPrivateContact(t, e, e.Rep1, email, email)
	}
	mail := seedCapturedMail(t, e, "queued@state.test", "a message the sweep can ask over")
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
		mail, contacts["queued@state.test"])
	for _, email := range []string{"decided@state.test", "classifying@state.test", "unsure@state.test"} {
		seedPendingDisposition(t, e, email, "state.test", mail)
	}
	e.WsExec(t, `UPDATE capture_pending_counterparty SET status = 'unsure' WHERE email = 'unsure@state.test'`)
	e.WsExec(t, `INSERT INTO capture_sender_override (user_id, address, decision, captured_by)
		VALUES ($1, 'decided@state.test', 'business', 'human:test')`, e.Rep1)

	listed, err := capture.ContactsAwaitingDecisionFor(ownerReader(e, e.Rep1), InstallationDB(e.Pool))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != len(want) {
		t.Fatalf("listed %d contacts, want the %d seeded", len(listed), len(want))
	}
	for _, c := range listed {
		if len(c.Addresses) != 1 {
			t.Fatalf("contact %s carries %d address states, want one per address", c.ContactID, len(c.Addresses))
		}
		got := c.Addresses[0]
		if got.State != want[got.Email] {
			t.Errorf("%s waits on %q, want %q", got.Email, got.State, want[got.Email])
		}
	}
}

// The states pair with the addresses by position, so a contact with two
// addresses must carry each address's own state.
func TestEachAddressOfAWaitingContactCarriesItsOwnState(t *testing.T) {
	e := integration.Setup(t)
	contact := seedOwnerPrivateContact(t, e, e.Rep1, "Two Addresses", "a@pair.test")
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, 'b@pair.test', 'capture', 'connector:gmail')`, contact)
	mail := seedCapturedMail(t, e, "b@pair.test", "a message about the second address")
	seedPendingDisposition(t, e, "b@pair.test", "pair.test", mail)

	listed, err := capture.ContactsAwaitingDecisionFor(ownerReader(e, e.Rep1), InstallationDB(e.Pool))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || len(listed[0].Addresses) != 2 {
		t.Fatalf("want one contact with two address states, got %+v", listed)
	}
	want := map[string]capture.AwaitingState{
		"a@pair.test": capture.AwaitingNothingToJudge,
		"b@pair.test": capture.AwaitingClassifier,
	}
	for _, got := range listed[0].Addresses {
		if got.State != want[got.Email] {
			t.Errorf("%s waits on %q, want %q", got.Email, got.State, want[got.Email])
		}
	}
}

// A decision counts as applying only where a pass will apply it. A contact a
// user worked on is out of both sweeps. The noise sweep keeps a sender the
// workspace writes to. Those decisions are left to the owner.
func TestADecisionNoPassAppliesIsLeftToTheOwner(t *testing.T) {
	e := integration.Setup(t)
	want := map[string]capture.AwaitingState{
		"untouched@kept.test":  capture.AwaitingYourAnswerApplying,
		"touched@kept.test":    capture.AwaitingNothingToJudge,
		"nomail@business.test": capture.AwaitingNothingToJudge,
		"written@kept.test":    capture.AwaitingNothingToJudge,
	}
	decisions := map[string]string{
		"untouched@kept.test":  "keep_out",
		"touched@kept.test":    "keep_out",
		"nomail@business.test": "business",
		"written@kept.test":    "keep_out",
	}
	for email, decision := range decisions {
		contact := seedOwnerPrivateContact(t, e, e.Rep1, email, email)
		e.WsExec(t, `INSERT INTO capture_sender_override (user_id, address, decision, captured_by)
			VALUES ($1, $2, $3, 'human:test')`, e.Rep1, email, decision)
		if email == "touched@kept.test" {
			e.WsExec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
				VALUES ('human', 'user:'||$1::text, 'update', 'contact', $2)`, e.Rep1, contact)
		}
	}

	// The noise sweep refuses to retract a sender the workspace writes to.
	seedOutboundMail(t, e, "written@kept.test", "thanks, let us proceed")

	listed, err := capture.ContactsAwaitingDecisionFor(ownerReader(e, e.Rep1), InstallationDB(e.Pool))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != len(want) {
		t.Fatalf("listed %d contacts, want the %d seeded", len(listed), len(want))
	}
	for _, c := range listed {
		got := c.Addresses[0]
		if got.State != want[got.Email] {
			t.Errorf("%s waits on %q, want %q", got.Email, got.State, want[got.Email])
		}
	}
}

// An `unsure` sender has a review offer, and no pass claims it. So the owner's
// answer must put it back in the queue, or it changes nothing they can see.
func TestAnOwnersAnswerAboutAnUnsureSenderIsAppliedByTheNextPass(t *testing.T) {
	e := integration.Setup(t)
	const email = "dana@answered.example"
	activityID := seedCapturedMail(t, e, email, "about your services")
	dispositionID := seedPendingDisposition(t, e, email, "answered.example", activityID)
	retireToUnsure(t, e, dispositionID)
	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{}, CaptureConfig{}, slog.Default())
	if err := seedLegacySenderReviews(t, e, engine); err != nil {
		t.Fatalf("staging the review offer: %v", err)
	}
	offer := stagedProposalID(t, e, dispositionID)

	owner := purgeCtx(e, e.Rep1)
	if _, err := newSenderOverrideStore(e.Pool).Set(owner, email, capture.OverrideBusiness); err != nil {
		t.Fatalf("recording the owner's answer: %v", err)
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusPending {
		t.Fatalf("the answered question is %q, want it back in the queue as pending", got)
	}
	if n := countIn(t, e, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'pending'`, offer); n != 0 {
		t.Fatal("the review offer still asks a question its owner already answered")
	}

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusReal {
		t.Fatalf("after the pass the question is %q, want real from the owner's answer", got)
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		 WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 1 {
		t.Fatalf("%d workspace contacts for the answered sender, want the one the answer creates", n)
	}
}

// The answer is the owner's about their own mailbox, so a colleague's open
// question about the same address is theirs to answer.
func TestAnOwnersAnswerLeavesAColleaguesQuestionAlone(t *testing.T) {
	e := integration.Setup(t)
	const email = "shared@answered.example"
	activityID := seedCapturedMail(t, e, email, "hello")
	dispositionID := seedPendingDisposition(t, e, email, "answered.example", activityID)
	retireToUnsure(t, e, dispositionID)

	colleague := purgeCtx(e, e.Rep3)
	if _, err := newSenderOverrideStore(e.Pool).Set(colleague, email, capture.OverrideBusiness); err != nil {
		t.Fatalf("recording the colleague's answer: %v", err)
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusUnsure {
		t.Fatalf("a colleague's answer moved the owner's question to %q", got)
	}
}
