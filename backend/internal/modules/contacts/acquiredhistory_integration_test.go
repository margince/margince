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

// sentBy is a message the rep's mailbox sent to the address, at a stated time.
func (e *dedupeEnv) sentBy(ctx context.Context, t *testing.T, email string, at time.Time) EnsureCounterpartyInput {
	t.Helper()
	in := e.outboundOnly(ctx, t, e.datedEnsureInput(ctx, t, email, "history.test", at))
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
	in := e.sentBy(ctx, t, "old@history.test", time.Now().AddDate(-3, 0, 0))

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
	in := e.sentBy(ctx, t, "new@history.test", time.Now().Add(-time.Hour))

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("a counterparty first written to after the connection is %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// A seat with no connection on record has no date to compare against, and the
// duty stays owed rather than being excused by a missing row.
func TestMailFromASeatWithNoConnectionExcusesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	in := e.sentBy(ctx, t, "noconn@history.test", time.Now().AddDate(-2, 0, 0))

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("mail from a seat with no connection made the contact %q, want %q", kind, AcquiredUnknownLegacy)
	}
}

// An inbound Date header is the sender's claim. A stranger copied on a
// backdated inbound message must not look like old correspondence of ours.
func TestABackdatedInboundCopyDoesNotMakeHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const email = "copied@history.test"
	e.connectMailbox(ctx, t, time.Now().Add(-24*time.Hour))
	backdated := e.datedEnsureInput(ctx, t, "spoofer@elsewhere.test", "elsewhere.test", time.Now().AddDate(-5, 0, 0))
	e.importedByRep(ctx, t, backdated.ActivityID)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'cc', $2)`,
			backdated.ActivityID, email)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	in := e.sentBy(ctx, t, email, time.Now().Add(-time.Hour))

	if kind := e.ensuredKind(ctx, t, in); kind != AcquiredUnknownLegacy {
		t.Errorf("a stranger copied on a backdated inbound mail is %q, want %q", kind, AcquiredUnknownLegacy)
	}
}
