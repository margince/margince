// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// HOW WIDELY a judged sender's contact record is visible.
//
// A `person` verdict says the sender is a named human. It does not say the
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
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindPerson}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// The record exists: the positive control, without which this test would
	// pass over a verdict that created nothing at all.
	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = $1`, email); n != 1 {
		t.Fatalf("%d persons for an address we wrote to, want the record to be made", n)
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
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
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindPerson}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 1 {
		t.Errorf("%d workspace-visible persons for an address that answered us, want 1", n)
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
// everybody reads names the person it was shut about.
func TestASenderOnAHeldThreadIsNotAnnouncedToTheWorkspace(t *testing.T) {
	e := integration.Setup(t)
	const email = "partner@confidential.example"
	activity := seedThreadedMail(t, e, email, "The matter", "inbound", "thr-held")
	seedThreadHold(t, e, "thr-held", "held")
	id := seedPendingDisposition(t, e, email, "confidential.example", activity)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindPerson}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = $1`, email); n != 1 {
		t.Fatalf("%d persons for a held thread's sender, want the record to be made", n)
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
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
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindPerson}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 1 {
		t.Errorf("%d workspace-visible persons on a cleared thread, want 1", n)
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
// obligation: the correspondence may not be republished, so the person on the
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
	brain := &scriptedVerdictBrain{verdicts: map[string]string{id.String(): capture.KindPerson}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = $1 AND p.visibility = 'workspace'`, email); n != 0 {
		t.Error("a restricted message's counterparty was announced to the workspace")
	}
}
