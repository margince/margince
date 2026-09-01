// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A card imports only while the message it arrived on may still be read.
//
// The check has to live INSIDE the card's own write transaction, and that is
// what these tests are for rather than the refusal itself. A check taken before
// the transaction refuses the same messages and passes the same assertions,
// while leaving the gap it appears to close — so the last test here narrows the
// message from a CONCURRENT transaction, holding the card's write open, which is
// the only arrangement the two placements answer differently.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mailedCard is one card, as it would arrive attached to a message.
const mailedCard = `BEGIN:VCARD
VERSION:3.0
N:Post;Petra;;;
FN:Petra Post
ORG:Briefkasten GmbH
TITLE:Head of Mail
EMAIL;TYPE=INTERNET:petra@briefkasten.example
TEL;TYPE=WORK,VOICE:+49 30 5556677
END:VCARD
`

// seedSourceMessage writes the message a mailed card arrived on, at the given
// audience, and answers its id.
func seedSourceMessage(ctx context.Context, t *testing.T, e *dedupeEnv, audience string) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, body, direction, occurred_at,
			                      source_system, source_id, source, captured_by, audience)
			VALUES ($1, 'email', 'my card', 'attached', 'inbound', now(),
			        'gmail', $2, 'gmail:test', 'connector:gmail', $3)`,
			activity, activity.String(), audience)
		return err
	}); err != nil {
		t.Fatalf("seeding the card's source message: %v", err)
	}
	return activity
}

// importMailedCard runs the mailed import against one source message.
func importMailedCard(ctx context.Context, t *testing.T, e *dedupeEnv, source ids.UUID) []VCardResult {
	t.Helper()
	entries, err := ParseVCards(strings.NewReader(mailedCard))
	if err != nil {
		t.Fatalf("parsing the card: %v", err)
	}
	results, err := e.store.ImportVCardsFromMessage(ctx, entries, source)
	if err != nil {
		t.Fatalf("importing from a message: %v", err)
	}
	return results
}

// Both arms of the plain case. The messages differ in nothing but the audience,
// so the admit arm is what makes the refusal evidence rather than a path that
// writes nothing at all.
func TestACardImportsOnlyFromAMessageTheWorkspaceMayRead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	narrowed := importMailedCard(ctx, t, e, seedSourceMessage(ctx, t, e, "participants"))
	if len(narrowed) != 1 || narrowed[0].Outcome != VCardSkipped {
		t.Fatalf("a card on a message the workspace may not read imported as %q, want skipped — "+
			"its contents would sit on a record every seat can open, and narrowing the "+
			"mail afterwards does not take them back", outcomeOf(narrowed))
	}

	open := importMailedCard(ctx, t, e, seedSourceMessage(ctx, t, e, "workspace"))
	if len(open) != 1 || open[0].Outcome != VCardCreated {
		t.Fatalf("a card on an open message imported as %q, want created — the refusal above "+
			"proves nothing if the import cannot write from a message it is allowed to",
			outcomeOf(open))
	}
}

// A message that is GONE lends nothing, and says so as a skip rather than an
// error: the mailed path's job would otherwise retry into the same missing row
// until it exhausted.
func TestACardImportsNothingWhenItsMessageHasBeenErased(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	results := importMailedCard(ctx, t, e, ids.NewV7())
	if len(results) != 1 || results[0].Outcome != VCardSkipped {
		t.Errorf("a card whose message no longer exists imported as %q, want skipped", outcomeOf(results))
	}
}

// The card's write and its liveness check are ONE transaction, proved by the
// lock they share.
//
// This is the placement test, and it works by observation rather than by racing:
// while another transaction holds the message under FOR UPDATE, the import must
// BLOCK. That can only happen if the import reads the message from inside a
// transaction it has already opened for the write — a check taken before the
// transaction would read, be refused or admitted, and return, never blocking a
// write that had not started.
//
// The gap this closes: with the check outside, a narrowing committing between
// the check and the write lands the card anyway. There is no "between" here,
// because the FOR SHARE is held from the first statement to the commit.
func TestTheCardsLivenessCheckIsInsideItsWriteTransaction(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	source := seedSourceMessage(ctx, t, e, "workspace")

	// Hold the message exclusively. A FOR SHARE inside the import must wait.
	holder, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the holding transaction: %v", err)
	}
	if _, err := holder.Exec(ctx, `SELECT id FROM activity WHERE id = $1 FOR UPDATE`, source); err != nil {
		t.Fatalf("holding the message: %v", err)
	}

	done := make(chan []VCardResult, 1)
	failed := make(chan error, 1)
	go func() {
		entries, perr := ParseVCards(strings.NewReader(mailedCard))
		if perr != nil {
			failed <- perr
			return
		}
		results, ierr := e.store.ImportVCardsFromMessage(ctx, entries, source)
		if ierr != nil {
			failed <- ierr
			return
		}
		done <- results
	}()

	// It must NOT finish while the message is held. If it does, its check ran
	// somewhere that takes no lock — which is the placement this test exists to
	// refuse.
	select {
	case results := <-done:
		t.Fatalf("the import finished as %q while the message was held exclusively — its "+
			"liveness check took no lock inside the write transaction, so a narrowing "+
			"committing before the write would land the card anyway", outcomeOf(results))
	case err := <-failed:
		t.Fatalf("importing while the message was held: %v", err)
	case <-time.After(500 * time.Millisecond):
		// Blocked, which is the whole assertion.
	}

	// Release it and let the import through, so the test also proves the block
	// was the lock and not a hang.
	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("releasing the message: %v", err)
	}
	select {
	case results := <-done:
		if len(results) != 1 || results[0].Outcome != VCardCreated {
			t.Errorf("after the lock was released the card imported as %q, want created",
				outcomeOf(results))
		}
	case err := <-failed:
		t.Fatalf("importing after the message was released: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the import never finished after the message was released")
	}
}

// outcomeOf names what one import answered, for a failure message that says what
// happened rather than that something did.
func outcomeOf(results []VCardResult) string {
	if len(results) == 0 {
		return "no result at all"
	}
	if results[0].Reason != "" {
		return string(results[0].Outcome) + " (" + results[0].Reason + ")"
	}
	return string(results[0].Outcome)
}
