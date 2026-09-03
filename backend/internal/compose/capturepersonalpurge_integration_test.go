// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// When a `personal` verdict destroys a founder's private mail, and — the half
// that matters — when it must not.
//
// This is the only path in the product where a classification leads to
// irreversible deletion with nobody in the loop at the moment it happens. Each
// test here pins one of the conditions that makes that survivable: the window is
// measured per message, a verdict nobody confirmed waits longer, and an owner's
// overrule cancels it outright.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestPersonalMailIsDestroyedOnceItsWindowCloses(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "mama@familie.example", "Sonntag Essen?", e.Rep1)
	resolvePersonal(t, e, "mama@familie.example", mail, e.Rep1, true)
	agePersonalMail(t, e, mail, "mama@familie.example", 8*24*time.Hour)

	if n := sweepPersonal(t, e); n != 1 {
		t.Fatalf("destroyed %d messages, want 1", n)
	}
	if body := activityBody(t, e, mail); body != "" {
		t.Fatalf("a family letter past its window kept its body %q", body)
	}
}

func TestPersonalMailInsideItsWindowSurvives(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "mama@familie.example", "Sonntag Essen?", e.Rep1)
	resolvePersonal(t, e, "mama@familie.example", mail, e.Rep1, true)
	agePersonalMail(t, e, mail, "mama@familie.example", 2*24*time.Hour)

	if n := sweepPersonal(t, e); n != 0 {
		t.Fatalf("destroyed %d messages, want 0 — the owner still has days to object", n)
	}
	if body := activityBody(t, e, mail); body == "" {
		t.Fatal("a message inside its undo window was destroyed")
	}
}

func TestAMessageThatArrivedAfterTheVerdictGetsItsOwnWindow(t *testing.T) {
	// The defect a verdict-keyed window ships: the sender keeps writing, and
	// mail that arrived after the decision would be destroyed on arrival with no
	// undo window at all — for exactly the messages a wrong verdict is most
	// likely to catch.
	e := integration.Setup(t)
	old := seedPurgeableMail(t, e, "mama@familie.example", "letzte Woche", e.Rep1)
	resolvePersonal(t, e, "mama@familie.example", old, e.Rep1, true)
	agePersonalMail(t, e, old, "mama@familie.example", 8*24*time.Hour)

	// A second message from the same sender, captured today. The verdict is old;
	// this message is not.
	fresh := seedPurgeableMail(t, e, "mama@familie.example", "heute", e.Rep1)

	if n := sweepPersonal(t, e); n != 1 {
		t.Fatalf("destroyed %d messages, want 1 — only the message whose own window closed", n)
	}
	if body := activityBody(t, e, fresh); body == "" {
		t.Fatal("a message that arrived after the verdict was destroyed with the older batch, " +
			"so it never had an undo window of its own")
	}
}

func TestAVerdictNobodyConfirmedWaitsLonger(t *testing.T) {
	// Two authorities reach the same verdict and they are not worth the same. A
	// person said so on purpose; the classifier guessed and nobody has looked.
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "arzt@praxis.example", "Befund", e.Rep1)
	resolvePersonal(t, e, "arzt@praxis.example", mail, e.Rep1, false)
	agePersonalMail(t, e, mail, "arzt@praxis.example", 8*24*time.Hour)

	if n := sweepPersonal(t, e); n != 0 {
		t.Fatalf("destroyed %d messages, want 0 — eight days is past the owner's window "+
			"but well inside the classifier's", n)
	}
	agePersonalMail(t, e, mail, "arzt@praxis.example", 31*24*time.Hour)
	if n := sweepPersonal(t, e); n != 1 {
		t.Fatalf("destroyed %d messages, want 1 once the longer window closed", n)
	}
}

func TestOverrulingTheVerdictCancelsTheDestruction(t *testing.T) {
	// The cancel that would silently not work: SenderOverrideStore.Set writes
	// capture_sender_override and an audit row and never touches this ledger, so
	// a selector that did not anti-join it would destroy the mail anyway while
	// the page showed the sender as business.
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "steuer@kanzlei.example", "Jahresabschluss", e.Rep1)
	resolvePersonal(t, e, "steuer@kanzlei.example", mail, e.Rep1, true)
	agePersonalMail(t, e, mail, "steuer@kanzlei.example", 8*24*time.Hour)
	overruleAsBusiness(t, e, e.Rep1, "steuer@kanzlei.example")

	if n := sweepPersonal(t, e); n != 0 {
		t.Fatalf("destroyed %d messages, want 0 — the owner said this sender is business", n)
	}
	if body := activityBody(t, e, mail); body == "" {
		t.Fatal("mail was destroyed for a sender the owner had overruled back to business")
	}
}

func TestAnotherSeatsPersonalMailIsNotDestroyed(t *testing.T) {
	// One rep's family member is another rep's customer. The verdict is per
	// seat, and so is the destruction.
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "mama@familie.example", "Sonntag Essen?", e.Rep1)
	addImporter(t, e, mail, e.Rep2)
	resolvePersonal(t, e, "mama@familie.example", mail, e.Rep1, true)
	agePersonalMail(t, e, mail, "mama@familie.example", 8*24*time.Hour)

	if n := sweepPersonal(t, e); n != 0 {
		t.Fatalf("destroyed %d messages, want 0 — a colleague imported this one too", n)
	}
	if body := activityBody(t, e, mail); body == "" {
		t.Fatal("a message a colleague also imported was destroyed to satisfy one seat's verdict")
	}
	if n := importCount(t, e, mail, e.Rep2); n != 1 {
		t.Fatalf("the colleague has %d import rows, want 1 — their claim survives", n)
	}
	// The release arm actually ran: this seat's own claim is gone, and the audit
	// row says who stopped being able to read it. Without both assertions the
	// test passes when carryOut releases nothing at all, which is the shape it
	// had before — "the activity survived" is also true when the sweep did
	// nothing whatsoever.
	if n := importCount(t, e, mail, e.Rep1); n != 0 {
		t.Errorf("the purging seat still holds %d import rows, want 0 — the release arm did not run", n)
	}
	if n := archiveAuditRows(t, e, mail); n != 1 {
		t.Errorf("%d archive audit rows for the released message, want 1: who stopped being able "+
			"to read this, and when, is exactly what a later access dispute asks", n)
	}
}

// archiveAuditRows counts what the release arm recorded about one message.
func archiveAuditRows(t *testing.T, e *integration.Env, activityID ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_log
			  WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'archive'`,
			activityID).Scan(&n)
	}); err != nil {
		t.Fatalf("counting archive audit rows: %v", err)
	}
	return n
}

func TestTheOwnersOwnRepliesSurviveAPersonalPurge(t *testing.T) {
	// The seat's own sent mail is its own record, and a stranger's forged header
	// must never reach it. capture_import rows are written whatever the
	// direction, so a selector that does not say `inbound` destroys every reply
	// the owner wrote to the address as well as the mail they received.
	e := integration.Setup(t)
	inbound := seedPurgeableMail(t, e, "mama@familie.example", "Sonntag Essen?", e.Rep1)
	reply := seedOutboundMail(t, e, "mama@familie.example", "Ja, gerne")
	addImporter(t, e, reply, e.Rep1)
	// Unattested, so this test asks about the DIRECTION clause alone. An
	// attested reply is refused by the corresponds-with-them clause too, and a
	// test that could not tell the two apart would pass with either one gone.
	unattest(t, e, reply)
	resolvePersonal(t, e, "mama@familie.example", inbound, e.Rep1, false)
	agePersonalMail(t, e, inbound, "mama@familie.example", 31*24*time.Hour)
	ageOneActivity(t, e, reply, 31*24*time.Hour)

	sweepPersonal(t, e)
	if body := activityBody(t, e, reply); body == "" {
		t.Fatal("the owner's own outbound reply was destroyed by a verdict about the SENDER")
	}
}

func TestMailThePersonRepliedToSurvivesAPersonalPurge(t *testing.T) {
	// Writing to an address is the T1 signal that they are a real counterparty,
	// and it is the documented recovery: reply to a wrongly judged sender and
	// the sweep lets go. Without it a wrong verdict has no way back, because a
	// personal verdict hides nothing and so surfaces nothing to object to.
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "steuer@kanzlei.example", "Jahresabschluss", e.Rep1)
	resolvePersonal(t, e, "steuer@kanzlei.example", mail, e.Rep1, false)
	agePersonalMail(t, e, mail, "steuer@kanzlei.example", 31*24*time.Hour)
	// The owner writes back, which is what a person does on noticing the record
	// is wrong. seedOutboundMail marks it attested, as the capture path does.
	seedOutboundMail(t, e, "steuer@kanzlei.example", "anbei die Unterlagen")

	if n := sweepPersonal(t, e); n != 0 {
		t.Fatalf("destroyed %d messages, want 0 — the workspace corresponds with this address", n)
	}
	if body := activityBody(t, e, mail); body == "" {
		t.Fatal("mail was destroyed from an address the workspace had written to, " +
			"so the reply-to-recover escape does not work")
	}
}

func TestMailArrivingLongAfterTheVerdictIsNotDestroyedByIt(t *testing.T) {
	// The forward bound. counterparty_email comes off an unauthenticated From
	// header: forge one message, have it judged personal, and without this
	// clause every later mail the REAL owner of that address sends is destroyed
	// — unbounded, and never seen by a human.
	e := integration.Setup(t)
	forged := seedPurgeableMail(t, e, "bigcustomer@corp.example", "privat", e.Rep1)
	resolvePersonal(t, e, "bigcustomer@corp.example", forged, e.Rep1, false)
	agePersonalMail(t, e, forged, "bigcustomer@corp.example", 60*24*time.Hour)

	// Real correspondence from that address, arriving well past the verdict's
	// reach but still old enough that the window alone would take it.
	later := seedPurgeableMail(t, e, "bigcustomer@corp.example", "Angebot 2026", e.Rep1)
	ageOneActivity(t, e, later, 31*24*time.Hour)

	sweepPersonal(t, e)
	if body := activityBody(t, e, later); body == "" {
		t.Fatal("mail arriving long after the verdict was destroyed by it: one forged header " +
			"reaches the real owner's correspondence without bound")
	}
}

// resolvePersonal puts a sender on the ledger and closes it as `personal`
// through the store that closes it in production.
//
// ResolveAs rather than an UPDATE: `resolved_by_owner` is the column under test
// in half these cases, and a test that wrote it itself would prove nothing about
// whether the engine writes it.
func resolvePersonal(t *testing.T, e *integration.Env, from string, activityID, owner ids.UUID, byOwner bool) {
	t.Helper()
	store := capture.NewPendingStore(InstallationDB(e.Pool))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_pending_counterparty
			       (email, domain, activity_id, owner_id, status, next_attempt_at)
			VALUES ($1, split_part($1, '@', 2), $2, $3, 'pending', now())`, from, activityID, owner)
		return err
	}); err != nil {
		t.Fatalf("seeding the pending sender: %v", err)
	}
	due, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming the pending sender: %v", err)
	}
	var row capture.PendingCounterparty
	for _, d := range due {
		if d.Email == from {
			row = d
		}
	}
	if row.ID == ids.Nil {
		t.Fatalf("the ledger handed out no row for %s", from)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		won, err := store.ResolveAs(e.Admin(), tx, row,
			capture.PendingStatusNoise, capture.KindPersonal, "test", byOwner,
			capture.VerdictMeasurement{})
		if err != nil {
			return err
		}
		if !won {
			t.Fatal("the resolve did not win its own claim")
		}
		return nil
	}); err != nil {
		t.Fatalf("resolving the sender as personal: %v", err)
	}
}

// unattest clears the outbound-correspondence flag, isolating the direction
// clause from the sibling clause that would otherwise also save the row.
func unattest(t *testing.T, e *integration.Env, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET counterparty_outbound_attested = false WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("clearing the outbound attestation: %v", err)
	}
}

// ageOneActivity backdates a single message without touching a verdict.
func ageOneActivity(t *testing.T, e *integration.Env, activityID ids.UUID, age time.Duration) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET created_at = now() - $2::interval WHERE id = $1`,
			activityID, age.String())
		return err
	}); err != nil {
		t.Fatalf("backdating a message: %v", err)
	}
}

// agePersonalMail backdates BOTH clocks the window reads, so a test asks about
// the age it means rather than about whichever of the two happens to be newer.
func agePersonalMail(t *testing.T, e *integration.Env, activityID ids.UUID, from string, age time.Duration) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`UPDATE activity SET created_at = now() - $2::interval WHERE id = $1`,
			activityID, age.String()); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_pending_counterparty SET resolved_at = now() - $2::interval WHERE email = $1`,
			from, age.String())
		return err
	}); err != nil {
		t.Fatalf("backdating the message and its verdict: %v", err)
	}
}

// overruleAsBusiness records the owner's cancel through the store a person's own
// click writes it with.
func overruleAsBusiness(t *testing.T, e *integration.Env, seat ids.UUID, address string) {
	t.Helper()
	store := capture.NewSenderOverrideStore(InstallationDB(e.Pool))
	if _, err := store.Set(purgeCtx(e, seat), address, capture.OverrideBusiness); err != nil {
		t.Fatalf("overruling the verdict: %v", err)
	}
}

func sweepPersonal(t *testing.T, e *integration.Env) int {
	t.Helper()
	purger := NewCapturePurger(e.Pool, NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.Default()))
	n, err := purger.SweepPersonalMail(e.Admin(), capture.DefaultPersonalPurgeWindows())
	if err != nil {
		t.Fatalf("sweeping personal mail: %v", err)
	}
	return n
}
