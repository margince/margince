// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// What the confidentiality classifier decides about a THREAD, and what the tier
// ladder is then allowed to create from it.
//
// The two lanes never spoke. A founder's aunt had twenty threads judged
// personal and held, and she was a contact in the shared CRM the whole time:
// one lane decided what could be read, the other decided what got created, and
// nothing carried the first answer to the second.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedPersonalThread lands the verdict row the classifier writes for a thread it
// judged to be the mailbox owner's own life.
func seedPersonalThread(t *testing.T, e *integration.SearchEnv, user ids.UUID, threadKey string, seen []string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		// The capture pass opens the row; the classifier answers it. Upsert so
		// the fixture models the answer rather than racing the pass that asks.
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, seen_addresses, resolved_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (thread_key, user_id) DO UPDATE
			   SET status = EXCLUDED.status, kind = EXCLUDED.kind,
			       seen_addresses = EXCLUDED.seen_addresses, resolved_at = EXCLUDED.resolved_at`,
			threadKey, user, capturemod.VerdictHeld, capturemod.ThreadKindPersonal, seen)
		return err
	}); err != nil {
		t.Fatalf("seeding the personal thread verdict: %v", err)
	}
}

// A thread judged the owner's private life creates nobody, however much the
// workspace has written to that address.
func TestAPrivateThreadMintsNoContact(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent
	const aunt = "aunt@family.example"

	// The exchange that would otherwise create on sight: the owner writes, and
	// she answers on the same thread.
	syncSent(t, map[string]bool{"fam1@myco.example": true},
		email(captureOwner, "", aunt, "fam1@myco.example", ""))
	seedPersonalThread(t, e, e.Rep1, "fam1@myco.example", []string{aunt})

	sync(t, email(aunt, "Aunt Anne", captureOwner, "fam1r@family.example", "fam1@myco.example"))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'aunt@family.example'`); n != 0 {
		t.Fatalf("%d persons for a private correspondent, want 0 — "+
			"the classifier already decided this is not the workspace's business", n)
	}
	// The mail is kept. A personal thread is already held to the people on it,
	// and refusing the record is not a reason to lose somebody's family mail.
	if n := countRows(t, e, `
		SELECT count(*) FROM activity WHERE source_id = 'fam1r@family.example'`); n != 1 {
		t.Fatalf("%d activities for the private message, want 1 — the mail is kept", n)
	}
	// The refusal SETTLES nothing about the address. The ledger holds one open
	// question, opened by the owner's own first send before any verdict existed,
	// and this pass neither answers it nor adds to it: the disposition ledger is
	// keyed on the address and the decision was about one thread.
	//
	// TestAPrivateThreadDoesNotSettleTheAddressForever is where that matters —
	// the same person, writing about business, still becomes a contact.
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		WHERE email = 'aunt@family.example' AND resolved_at IS NOT NULL`); n != 0 {
		t.Fatalf("%d SETTLED ledger rows for a private correspondent, want 0 — "+
			"a thread's answer must not close an address's question", n)
	}
}

// The same person, writing about business on a thread nobody judged personal,
// is an ordinary counterparty.
//
// This is what the missing ledger row buys. Had the private thread settled the
// ADDRESS, an aunt who later sends a genuine enquiry — or a customer who once
// wrote about something private — would be refused forever by a decision that
// was never about them as a sender.
func TestAPrivateThreadDoesNotSettleTheAddressForever(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent
	const both = "cousin@family.example"

	syncSent(t, map[string]bool{"mix1@myco.example": true},
		email(captureOwner, "", both, "mix1@myco.example", ""))
	seedPersonalThread(t, e, e.Rep1, "mix1@myco.example", []string{both})
	sync(t, email(both, "Cousin", captureOwner, "mix1r@family.example", "mix1@myco.example"))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'cousin@family.example'`); n != 0 {
		t.Fatalf("%d persons after the private thread, want 0 — the fixture never reaches the case under test", n)
	}

	// A second, unjudged thread: they write about work and the owner answers.
	syncSent(t, map[string]bool{"mix2@myco.example": true},
		email(captureOwner, "", both, "mix2@myco.example", ""))
	sync(t, email(both, "Cousin", captureOwner, "mix2r@family.example", "mix2@myco.example"))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'cousin@family.example'`); n != 1 {
		t.Fatalf("%d persons after a business thread, want 1 — one private conversation "+
			"must not refuse somebody forever", n)
	}
}

// Held for a reason that is not `personal` is business the workspace conducts
// privately, and its parties are genuine contacts. A lawyer on a legal thread is
// somebody the workspace deals with; the hold is about who may READ the mail.
func TestAThreadHeldForBusinessReasonsStillMintsItsContact(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent
	const counsel = "counsel@kanzlei.example"

	syncSent(t, map[string]bool{"leg1@myco.example": true},
		email(captureOwner, "", counsel, "leg1@myco.example", ""))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, seen_addresses, resolved_at)
			VALUES ($1, $2, 'held', 'legal', $3, now())
			ON CONFLICT (thread_key, user_id) DO UPDATE
			   SET status = EXCLUDED.status, kind = EXCLUDED.kind,
			       seen_addresses = EXCLUDED.seen_addresses, resolved_at = EXCLUDED.resolved_at`,
			"leg1@myco.example", e.Rep1, []string{counsel})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sync(t, email(counsel, "Counsel", captureOwner, "leg1r@kanzlei.example", "leg1@myco.example"))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'counsel@kanzlei.example'`); n != 1 {
		t.Fatalf("%d persons for counsel on a legal thread, want 1 — "+
			"a legal hold is about who may read the mail, not about whether they are a contact", n)
	}
}

// The refusal is bound to the addresses the classifier SAW, not to the thread
// key alone.
//
// thread_key is the message's own References root, so a sender picks it
// verbatim. Without the binding, anybody who learns or provokes a personal
// thread key could write from a fresh address onto that root and be refused a
// record — quietly keeping themselves out of the CRM by borrowing somebody
// else's private conversation.
func TestAStrangerCannotBorrowSomebodyElsesPrivateThread(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync, syncSent := env.e, env.sync, env.syncSent
	const family = "sibling@family.example"
	const stranger = "vendor@supplier.example"

	// A genuine private conversation, judged personal.
	syncSent(t, map[string]bool{"borrow1@myco.example": true},
		email(captureOwner, "", family, "borrow1@myco.example", ""))
	seedPersonalThread(t, e, e.Rep1, "borrow1@myco.example", []string{family})

	// The vendor is a genuine counterparty on their OWN thread.
	syncSent(t, map[string]bool{"borrow2@myco.example": true},
		email(captureOwner, "", stranger, "borrow2@myco.example", ""))
	sync(t, email(stranger, "Vendor", captureOwner, "borrow2r@supplier.example", "borrow2@myco.example"))
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'vendor@supplier.example'`); n != 1 {
		t.Fatalf("%d persons for a vendor the workspace exchanged mail with, want 1 — "+
			"the fixture never reaches the case under test", n)
	}

	// They then write again, forging the family's private root onto it. The
	// message is captured; what must not happen is that it reads as part of
	// somebody else's private conversation.
	sync(t, email(stranger, "Vendor", captureOwner, "borrow3r@supplier.example", "borrow1@myco.example"))

	private, err := readPrivacyOfCapture(t, e, "borrow3r@supplier.example")
	if err != nil {
		t.Fatal(err)
	}
	if private {
		t.Error("a stranger's message inherited a private thread they were never part of")
	}
}

// readPrivacyOfCapture answers whether the ladder read one captured message as
// belonging to a private conversation, by the trace reason it recorded.
func readPrivacyOfCapture(t *testing.T, e *integration.SearchEnv, sourceID string) (bool, error) {
	t.Helper()
	var private bool
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT EXISTS (
			  SELECT 1 FROM capture_trace
			   WHERE source_id = $1 AND reason = $2)`,
			sourceID, capturemod.TraceReasonPrivateThread).Scan(&private)
	})
	return private, err
}
