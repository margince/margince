// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// HOW WIDELY a judged sender's contact record is visible.
//
// A `contact` verdict says the sender is a named human. It does not say the
// workspace should be told, and three situations turn on that difference: mail
// we sent to somebody who never answered, a thread under a confidentiality
// hold, and a message under a legal restriction. Each keeps the record — the
// owner did correspond with somebody real — and keeps it theirs.
//
// Every case below is paired with its opposite, so a narrowing that never
// widened again would fail here rather than read as a pass.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An address the mailbox owner WROTE TO, which has never answered, becomes the
// owner's contact rather than the workspace's.
//
// Writing to somebody is an intention, not a relationship. Nobody wrote in,
// nothing is owed, and publishing the record tells every colleague who this rep
// is prospecting. The record is still made — the owner did correspond with a
// real address — and it widens on its own terms if the address ever answers.
func TestAnAddressWeWroteToThatNeverAnsweredStaysTheOwners(t *testing.T) {
	e := integration.Setup(t)
	const email = "desk@citygarden.example"
	activity := seedOutboundMail(t, e, email, "Access card")
	id := seedPendingDisposition(t, e, email, "citygarden.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// The record exists: the positive control, without which this test would
	// pass over a verdict that created nothing at all.
	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1`, email); n != 1 {
		t.Fatalf("%d contacts for an address we wrote to, want the record to be made", n)
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 0 {
		t.Error("an address we wrote to that never answered was published to the workspace")
	}
}

// The same address, once it has answered, is the workspace's.
//
// This is the half that keeps the narrowing from being a permanent hiding
// place: a reply is what turns an intention into a relationship, and a rep
// whose prospect answers should not have to publish the contact by hand.
func TestAnAddressThatAnsweredUsIsTheWorkspacesContact(t *testing.T) {
	e := integration.Setup(t)
	const email = "buyer@prospect.example"
	// We wrote first, and they replied — both messages on one thread, which is
	// what wroteBackTx reads.
	activity := seedThreadedMail(t, e, email, "Our proposal", "outbound", "thr-answered")
	seedThreadedMail(t, e, email, "Re: Our proposal", "inbound", "thr-answered")
	id := seedPendingDisposition(t, e, email, "prospect.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 1 {
		t.Errorf("%d workspace-visible contacts for an address that answered us, want 1", n)
	}
}

// seedThreadedMail plants one message in a direction and, optionally, on a named
// thread — the shape "has this address ever written back" is read from.
func seedThreadedMail(t *testing.T, e *integration.Env, counterparty, subject, direction, threadKey string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, raw, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, thread_key,
			                      counterparty_outbound_attested)
			VALUES ($1, 'email', $2, 'the message body', '{"headers":"…"}'::jsonb, $3,
			        'gmail', $4, 'gmail:'||$4, 'connector:gmail', $5, $6, $7)`,
			id, subject, direction, "vrd-"+id.String(), counterparty, threadKey,
			// The provider's own attestation that WE sent it. wroteBackTx
			// requires it on our side of the thread and is right to: an
			// unattested outbound row is one a sender could have caused, and
			// reading a reply off it would let anybody promote themselves into
			// the shared CRM by answering a message we never sent.
			direction == "outbound")
		return err
	})
	if err != nil {
		t.Fatalf("seeding %s mail: %v", direction, err)
	}
	return id
}

// A sender on a thread the mailbox is HOLDING keeps their record owner-scoped.
//
// The hold is a statement about who may read the correspondence. A
// workspace-visible contact minted off that thread announces the counterparty
// the hold exists to keep quiet: the mail stays shut while a row on a surface
// everybody reads names the contact it was shut about.
func TestASenderOnAHeldThreadIsNotAnnouncedToTheWorkspace(t *testing.T) {
	e := integration.Setup(t)
	const email = "partner@confidential.example"
	activity := seedThreadedMail(t, e, email, "The matter", "inbound", "thr-held")
	seedThreadHold(t, e, "thr-held", "held")
	id := seedPendingDisposition(t, e, email, "confidential.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1`, email); n != 1 {
		t.Fatalf("%d contacts for a held thread's sender, want the record to be made", n)
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 0 {
		t.Error("the counterparty of a held thread was announced to the workspace")
	}
}

// The same sender on an ordinary CLEARED thread is the workspace's, which is
// what keeps the test above from passing over a narrowing that never widens.
func TestASenderOnAClearedThreadIsTheWorkspacesContact(t *testing.T) {
	e := integration.Setup(t)
	const email = "partner@ordinary.example"
	activity := seedThreadedMail(t, e, email, "The matter", "inbound", "thr-cleared")
	seedThreadHold(t, e, "thr-cleared", "cleared")
	id := seedPendingDisposition(t, e, email, "ordinary.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 1 {
		t.Errorf("%d workspace-visible contacts on a cleared thread, want 1", n)
	}
}

// seedThreadHold records one thread's confidentiality verdict.
func seedThreadHold(t *testing.T, e *integration.Env, threadKey, status string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status)
			VALUES ($1, $2, $3)`, threadKey, e.Rep1, status)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the %s thread verdict: %v", status, err)
	}
}

// A RESTRICTED message never announces its counterparty either.
//
// The restriction and the thread hold are different mechanisms and the same
// obligation: the correspondence may not be republished, so the contact on the
// far end of it may not be named on a surface everybody reads. Held separately
// because a restriction can land on a message whose thread carries no verdict
// row at all.
func TestARestrictedMessageDoesNotAnnounceItsCounterparty(t *testing.T) {
	e := integration.Setup(t)
	const email = "counsel@restricted.example"
	activity := seedThreadedMail(t, e, email, "The matter", "inbound", "thr-restricted")
	// The whole shape a restriction takes. The database refuses a partial one:
	// a hold with no evidence of what qualified it is a hold nobody can answer
	// for, which is the point of the trigger.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_retention_evidence
			       (activity_id, basis, qualified_at, decided_by_name, reason)
			VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`,
			activity); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			UPDATE activity
			   SET restricted_at = now(), archived_at = now(),
			       restricted_reason = 'litigation hold',
			       restricted_until = now() + interval '1 year',
			       retention_class = 'commercial_correspondence', retention_class_at = now()
			 WHERE id = $1`, activity)
		return err
	}); err != nil {
		t.Fatalf("placing the message under a hold: %v", err)
	}
	id := seedPendingDisposition(t, e, email, "restricted.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 0 {
		t.Error("a restricted message's counterparty was announced to the workspace")
	}
}

// A message held by anything OTHER than a thread verdict still withholds its
// counterparty.
//
// A [Confidential] subject marker, a counterparty hold, and a mailbox that
// holds everything each hold a message without writing a thread-verdict row —
// the birth decision records them on the message's own audience instead. Asking
// only the thread ledger answered "not held" for all three, and published the
// counterparty of a confidential conversation while the mail itself stayed
// shut.
func TestAMessageHeldWithoutAThreadVerdictStillWithholdsItsCounterparty(t *testing.T) {
	e := integration.Setup(t)
	const email = "counsel@marked.example"
	activity := seedThreadedMail(t, e, email, "[Confidential] The matter", "inbound", "thr-marked")
	// The audience AND the reason the birth decision writes for a message held
	// by its subject marker. Both, because the reason is what separates a hold
	// from a message merely waiting for its own verdict — and no thread verdict
	// row is seeded, which is the whole point: this is the state the previous
	// reader could not see.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET audience = 'participants',
			                     audience_reason = 'explicitly_confidential'
			  WHERE id = $1`, activity)
		return err
	}); err != nil {
		t.Fatalf("holding the message: %v", err)
	}
	id := seedPendingDisposition(t, e, email, "marked.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 0 {
		t.Error("a held message's counterparty was announced to the workspace")
	}
}

// A withheld contact stays withheld: the ledger says so, and the readers that
// treat a contact verdict as permission to publish honour it.
//
// This is the half that was missing. The decision was recorded on the contact
// row, and two readers ask the LEDGER instead — the sweep that reopens held
// mail, and the birth decision that shares a future message from a sender
// already judged a contact. Both matched a contact deliberately kept private, so
// the next pass republished what this one withheld.
func TestAWithheldContactIsMarkedOnTheLedgerTheOtherReadersConsult(t *testing.T) {
	e := integration.Setup(t)
	const email = "desk@withheld.example"
	activity := seedThreadedMail(t, e, email, "Access card", "outbound", "thr-withheld")
	id := seedPendingDisposition(t, e, email, "withheld.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// The verdict IS contact — the positive control. Without it this test would
	// pass over a row that was never judged a contact at all, which is a
	// different reason for the readers to skip it.
	if n := countIn(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		 WHERE id = $1 AND status = 'real' AND kind = 'contact'`, id); n != 1 {
		t.Fatalf("the ledger does not record a contact verdict, so this proves nothing about withholding")
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		 WHERE id = $1 AND withheld_from_workspace`, id); n != 1 {
		t.Error("the contact was withheld and the ledger does not say so — the widening " +
			"sweep and the birth decision will both read it as permission to publish")
	}
}

// The ordinary published contact does NOT carry the mark, so the flag means
// something rather than being set on everything.
func TestAPublishedContactIsNotMarkedWithheld(t *testing.T) {
	e := integration.Setup(t)
	const email = "buyer@published.example"
	activity := seedThreadedMail(t, e, email, "Your proposal", "inbound", "thr-published")
	id := seedPendingDisposition(t, e, email, "published.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		 WHERE id = $1 AND withheld_from_workspace`, id); n != 0 {
		t.Error("an ordinary published contact is marked withheld")
	}
}

// An UNATTESTED outbound row makes no claim about who wrote.
//
// mailmap decides "outbound" by comparing the From header to the mailbox
// owner, and a sender writes their own From. So a forged From would otherwise
// have the prompt assert that the mailbox owner wrote a message they never
// sent — and the narrowing treat a stranger's mail as our own unanswered
// outreach. The provider's attestation is the difference, and without it the
// direction is reported as unknown rather than guessed.
func TestAnUnattestedOutboundRowMakesNoClaimAboutWhoWrote(t *testing.T) {
	e := integration.Setup(t)
	const email = "stranger@forged.example"
	// Outbound by the header, with no provider attestation behind it.
	activity := seedThreadedMail(t, e, email, "Re: your enquiry", "outbound", "thr-forged")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET counterparty_outbound_attested = false WHERE id = $1`, activity)
		return err
	}); err != nil {
		t.Fatalf("removing the attestation: %v", err)
	}
	store := capture.NewPendingStore(InstallationDB(e.Pool))
	seedPendingDisposition(t, e, email, "forged.example", activity)

	claimed, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming the due sender: %v", err)
	}

	var found bool
	for _, row := range claimed {
		if row.Email != email {
			continue
		}
		found = true
		if row.Direction != "" {
			t.Errorf("direction = %q for an unattested outbound row, want it unclaimed — "+
				"the From header is written by the sender", row.Direction)
		}
	}
	if !found {
		t.Fatal("the seeded sender was not claimed, so this asserts nothing")
	}
}

// The same row WITH the attestation does report outbound, so the check above
// is not passing because the direction never travels at all.
func TestAnAttestedOutboundRowReportsItsDirection(t *testing.T) {
	e := integration.Setup(t)
	const email = "prospect@attested.example"
	activity := seedThreadedMail(t, e, email, "Our proposal", "outbound", "thr-attested")
	store := capture.NewPendingStore(InstallationDB(e.Pool))
	seedPendingDisposition(t, e, email, "attested.example", activity)

	claimed, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming the due sender: %v", err)
	}

	var found bool
	for _, row := range claimed {
		if row.Email != email {
			continue
		}
		found = true
		if row.Direction != "outbound" {
			t.Errorf("direction = %q for an attested outbound row, want %q", row.Direction, "outbound")
		}
	}
	if !found {
		t.Fatal("the seeded sender was not claimed, so this asserts nothing")
	}
}

// A message narrow only because its OWN verdict has not come back is not a
// hold, and its counterparty is published like any other.
//
// This is the distinction that matters most here. Mail sits at `participants`
// while the sender is being judged — that is the question this very pass
// answers — and reading it as a confidentiality hold would narrow every
// ordinary contact the verdict was about to publish. The audience alone cannot
// tell the two apart; the reason can.
func TestAMessageWaitingForItsOwnVerdictIsNotAHold(t *testing.T) {
	e := integration.Setup(t)
	const email = "buyer@waiting.example"
	activity := seedThreadedMail(t, e, email, "Your proposal", "inbound", "thr-waiting")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET audience = 'participants',
			                     audience_reason = 'pending_verdict'
			  WHERE id = $1`, activity)
		return err
	}); err != nil {
		t.Fatalf("holding the message pending its verdict: %v", err)
	}
	id := seedPendingDisposition(t, e, email, "waiting.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindContact}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 1 {
		t.Errorf("%d workspace-visible contacts for a sender whose mail was merely awaiting "+
			"this verdict, want 1 — waiting for an answer is not a hold", n)
	}
}
