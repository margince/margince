// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// connectMailbox records that the rep attached a mailbox at a stated time.
func (e *dedupeEnv) connectMailbox(ctx context.Context, t *testing.T, at time.Time) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_connection (provider, user_id, status, created_at)
			VALUES ('gmail', $1, 'connected', $2)`, e.rep, at)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// importedByRep records the rep's mailbox as the one that delivered a message,
// the row capture writes for every message it imports.
func (e *dedupeEnv) importedByRep(ctx context.Context, t *testing.T, activity ids.ActivityID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, activity, e.rep)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// sentBy is a message a seat's mailbox sent to the address at a stated time,
// filed by the provider as sent and imported from that seat's mailbox.
func (e *dedupeEnv) sentBy(
	ctx context.Context, t *testing.T, sender ids.UUID, email string, at time.Time,
) EnsureCounterpartyInput {
	t.Helper()
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, email, "history.test", at))
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE activity SET counterparty_email = $2, counterparty_outbound_attested = true
			 WHERE id = $1`, in.ActivityID, email); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, 'from', $2)`,
			in.ActivityID, sender)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.importedByRep(ctx, t, in.ActivityID)
	return in
}

func (e *dedupeEnv) ensuredKind(ctx context.Context, t *testing.T, in EnsureCounterpartyInput) string {
	t.Helper()
	res, err := e.store.EnsureCounterparty(ctx, in)
	if err != nil || !res.ContactCreated {
		t.Fatalf("ensure = %+v (err %v), want a created contact", res, err)
	}
	kind, _ := acquisitionOf(ctx, t, e.store, res.ContactID)
	return kind
}

func TestSomebodyWeWroteToBeforeTheMailboxWasConnectedIsMailboxHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.connectMailbox(ctx, t, time.Now().Add(-24*time.Hour))
	in := e.sentBy(ctx, t, e.rep, "old@history.test", time.Now().AddDate(-3, 0, 0))

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredMailboxHistory {
		t.Errorf("a counterparty written to three years before the mailbox was connected is %q, want %q",
			kind, AcquiredMailboxHistory)
	}
}

// Mail sent after the connection is a new acquisition, and keeps the duty.
func TestSomebodyWeFirstWroteToAfterConnectingStaysUnknown(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.connectMailbox(ctx, t, time.Now().Add(-48*time.Hour))
	in := e.sentBy(ctx, t, e.rep, "new@history.test", time.Now().Add(-time.Hour))

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("a counterparty first written to after the connection is %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// A seat with no connection on record has no date to compare against, and the
// duty stays owed rather than being excused by a missing row.
func TestMailFromASeatWithNoConnectionExcusesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	in := e.sentBy(ctx, t, e.rep, "noconn@history.test", time.Now().AddDate(-2, 0, 0))

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("mail from a seat with no connection made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// A message whose From names our seat but which the provider never filed as
// sent is a header claim: anybody can write that From line and any Date.
func TestAnUnattestedOutboundDoesNotMakeHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.connectMailbox(ctx, t, time.Now().Add(-24*time.Hour))
	in := e.sentBy(ctx, t, e.rep, "spoofed@history.test", time.Now().AddDate(-5, 0, 0))
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE activity SET counterparty_outbound_attested = false WHERE id = $1`, in.ActivityID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("an outbound the provider never filed as sent made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// The connection that counts is the SENDER's. Mail a colleague sent that only
// reached the rep's mailbox says nothing about when the colleague connected.
func TestMailAnotherSeatSentIsNotTheImportersHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var colleague ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO app_user (email, display_name) VALUES ('colleague@history.test', 'Colleague')
			RETURNING id`).Scan(&colleague)
	}); err != nil {
		t.Fatal(err)
	}
	e.connectMailbox(ctx, t, time.Now().Add(-24*time.Hour))
	in := e.sentBy(ctx, t, colleague, "theirs@history.test", time.Now().AddDate(-1, 0, 0))

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("mail another seat sent made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// copiedOn is a message the rep's mailbox RECEIVED from somebody else that
// named the address on Cc. dated is its Date header, which the sender writes;
// arrived is when the provider says it reached the rep's mailbox.
func (e *dedupeEnv) copiedOn(
	ctx context.Context, t *testing.T, email string, dated, arrived time.Time, bulk bool,
) EnsureCounterpartyInput {
	t.Helper()
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, direction, occurred_at, source_system, source_id, source,
			                      captured_by, counterparty_email, bulk_mail_attested)
			VALUES ($1, 'email', 'hi', 'inbound', $2, 'gmail', $3, 'gmail:seed', 'connector:gmail',
			        'sender@elsewhere.test', $4)`,
			activityID, dated, activityID.String(), bulk); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, role, address)
			VALUES ($1, 'from', 'sender@elsewhere.test'), ($1, 'cc', $2)`, activityID, email); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_import (activity_id, user_id, provider_received_at) VALUES ($1, $2, $3)`,
			activityID, e.rep, arrived)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return EnsureCounterpartyInput{
		Email: email, Domain: "elsewhere.test",
		OwnerID: e.rep, ActivityID: activityID,
		Source: "gmail:" + activityID.String(), CapturedBy: "agent:capture_counterparty_verdict",
	}
}

// Somebody a third party copied on mail the rep's mailbox already held before it
// was connected was already in the company's correspondence.
func TestSomebodyCopiedOnMailTheMailboxAlreadyHeldIsMailboxHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.connectMailbox(ctx, t, time.Now().Add(-24*time.Hour))
	old := time.Now().AddDate(-3, 0, 0)
	in := e.copiedOn(ctx, t, "copied@elsewhere.test", old, old, false)

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredMailboxHistory {
		t.Errorf("an address on Cc of mail received three years before the connection is %q, want %q",
			kind, AcquiredMailboxHistory)
	}
}

// The same message arriving after the connection is a new acquisition.
func TestSomebodyCopiedOnMailReceivedAfterConnectingStaysUnknown(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.connectMailbox(ctx, t, time.Now().Add(-48*time.Hour))
	recent := time.Now().Add(-time.Hour)
	in := e.copiedOn(ctx, t, "copiednew@elsewhere.test", recent, recent, false)

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("an address on Cc of mail received after the connection is %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// A sender can write any Date header. Only the provider's arrival time counts,
// so a backdated message that arrived after the connection excuses nothing.
func TestABackdatedHeaderDoesNotMakeReceivedMailHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.connectMailbox(ctx, t, time.Now().Add(-48*time.Hour))
	in := e.copiedOn(ctx, t, "backdated@elsewhere.test", time.Now().AddDate(-5, 0, 0), time.Now().Add(-time.Hour), false)

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("a message dated years back that arrived after the connection made the contact %q, want %q",
			kind, AcquiredUnknownLegacy)
	}
}

// A list copying everybody is not correspondence the company held with them.
func TestBulkMailTheMailboxAlreadyHeldDoesNotMakeHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.connectMailbox(ctx, t, time.Now().Add(-24*time.Hour))
	old := time.Now().AddDate(-2, 0, 0)
	in := e.copiedOn(ctx, t, "listed@elsewhere.test", old, old, true)

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("bulk mail held before the connection made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// Somebody the seat copied on its own mail is as much a correspondent as the
// one it was addressed to: the company's mailbox held them both.
func TestSomebodyWeCopiedBeforeTheMailboxWasConnectedIsMailboxHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.connectMailbox(ctx, t, time.Now().Add(-24*time.Hour))
	in := e.sentBy(ctx, t, e.rep, "addressed@history.test", time.Now().AddDate(-3, 0, 0))
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'cc', 'copied@history.test')`,
			in.ActivityID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	in.Email = "copied@history.test"

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredMailboxHistory {
		t.Errorf("an address the seat copied three years before the connection is %q, want %q",
			kind, AcquiredMailboxHistory)
	}
}
