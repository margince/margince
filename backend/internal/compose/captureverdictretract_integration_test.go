// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// How far a noise verdict reaches into the PEOPLE list. Hiding a junk sender's
// mail while their contact stands leaves "receipts@" a person forever, so the
// verdict retracts the capture-only record too — bounded exactly like the rest
// of its effects: never a corresponded sender's record, never one a human
// touched, and a `keep out` only the decider's own.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The verdict arrives after the contact: a sender judged transactional today
// may have been minted a person under an earlier, looser creation rule, and
// hiding their mail alone would leave that record standing for good.
func TestANoiseVerdictRetractsTheContactItsSenderAlreadyHad(t *testing.T) {
	e := integration.Setup(t)
	const junk = "receipts@spesen.example"
	personID := seedCaptureOnlyContact(t, e, junk, e.Rep1)
	mail := seedCapturedMail(t, e, junk, "Ihre Abrechnung")
	dispositionID := seedPendingDisposition(t, e, junk, "spesen.example", mail)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindTransactional}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `SELECT count(*) FROM person WHERE id = $1 AND archived_at IS NOT NULL`, personID); n != 1 {
		t.Fatal("the noise verdict hid the mail but left the sender's contact standing")
	}
	// The retraction is an archive like any other: recoverable, and on the
	// record's own trail.
	if n := countIn(t, e, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'person' AND entity_id = $1 AND action = 'archive'`, personID); n != 1 {
		t.Fatal("the retraction left no audit row — an invisible archive is not recoverable")
	}
}

// An address the workspace has provably written to is a counterparty whatever
// the classifier called one message, and their record is not the verdict's to
// take — the same bound the domain suppression draws.
func TestANoiseVerdictLeavesACorrespondedSendersContact(t *testing.T) {
	e := integration.Setup(t)
	// A named localpart: a role address (billing@, support@) settles on the
	// deterministic rung and would never reach the noise arm this test is about.
	const supplier = "anna.mueller@lieferant.example"
	personID := seedCaptureOnlyContact(t, e, supplier, e.Rep1)
	seedOutboundMail(t, e, supplier, "please adjust our invoice")
	mail := seedCapturedMail(t, e, supplier, "monthly statement")
	dispositionID := seedPendingDisposition(t, e, supplier, "lieferant.example", mail)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindTransactional}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusNoise {
		t.Fatalf("the row settled %q, want noise — the fixture no longer exercises the noise arm at all", got)
	}

	if n := countIn(t, e, `SELECT count(*) FROM person WHERE id = $1 AND archived_at IS NULL`, personID); n != 1 {
		t.Fatal("a corresponded sender's contact was retracted on a classifier's word about one message")
	}
}

// A `keep out` is a statement about the decider's own mailbox. It may retract
// the record minted for THEM; a colleague who captured the same address keeps
// theirs, exactly as their mail keeps arriving.
func TestAKeepOutRetractsOnlyTheDecidersOwnContact(t *testing.T) {
	e := integration.Setup(t)
	const sender = "noreply@portal.example"
	colleagues := seedCaptureOnlyContact(t, e, sender, e.Rep2)
	mail := seedCapturedMail(t, e, sender, "password reset")
	// The pending row rides Rep1's mailbox, and Rep1 has already said keep out.
	dispositionID := seedPendingDisposition(t, e, sender, "portal.example", mail)
	seedSenderOverride(t, e, e.Rep1, sender, "keep_out")

	// No scripted verdict: the owner's decision answers before any model.
	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusNoise {
		t.Fatalf("the keep_out row settled %q, want noise — the fixture no longer reaches the owner-decision path", got)
	}

	if n := countIn(t, e, `SELECT count(*) FROM person WHERE id = $1 AND archived_at IS NULL`, colleagues); n != 1 {
		t.Fatal("one rep's keep_out retracted a COLLEAGUE's contact — a per-mailbox decision reached another seat's record")
	}
}

// A settled sender never re-enters the ledger, so contacts left standing by a
// noise verdict that predates the verdict-time retraction have no future
// verdict to catch them. The nightly reconcile is what does — and it honours a
// standing keep_out the same way, while a later `real` verdict outranks the
// stale noise row.
func TestTheReconcileSweepRetractsContactsANoiseVerdictAlreadyCovered(t *testing.T) {
	e := integration.Setup(t)
	covered := seedCaptureOnlyContact(t, e, "news@blast.example", e.Rep1)
	seedSettledNoiseRow(t, e, "news@blast.example", capture.KindNewsletter)

	keptOut := seedCaptureOnlyContact(t, e, "receipts@tool.example", e.Rep1)
	seedSenderOverride(t, e, e.Rep1, "receipts@tool.example", "keep_out")

	contested := seedCaptureOnlyContact(t, e, "mixed@vendor.example", e.Rep1)
	seedSettledNoiseRow(t, e, "mixed@vendor.example", capture.KindTransactional)
	seedSettledRow(t, e, "mixed@vendor.example", capture.PendingStatusReal, capture.KindPerson)

	// Written to SINCE the verdict: the one bound that can change after it.
	answered := seedCaptureOnlyContact(t, e, "kontakt.person@firma.example", e.Rep1)
	seedSettledNoiseRow(t, e, "kontakt.person@firma.example", capture.KindSpam)
	seedOutboundMail(t, e, "kontakt.person@firma.example", "thanks, let us proceed")

	// The owner has said `business` — their standing decision, which no
	// machine verdict outranks.
	readmitted := seedCaptureOnlyContact(t, e, "vertrieb.partner@haendler.example", e.Rep1)
	seedSettledNoiseRow(t, e, "vertrieb.partner@haendler.example", capture.KindSpam)
	seedSenderOverride(t, e, e.Rep1, "vertrieb.partner@haendler.example", "business")

	worker := NewLinkReconcileWorkspaceWorkerForTest(e.Pool, people.NewStore(InstallationDB(e.Pool)))
	if err := worker.reconcileLinksForWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	if n := countIn(t, e, `SELECT count(*) FROM person WHERE id = $1 AND archived_at IS NOT NULL`, covered); n != 1 {
		t.Fatal("a contact whose sender settled as noise before the retraction shipped was never cleaned up")
	}
	if n := countIn(t, e, `SELECT count(*) FROM person WHERE id = $1 AND archived_at IS NOT NULL`, keptOut); n != 1 {
		t.Fatal("a standing keep_out left its sender's contact in the people list")
	}
	if n := countIn(t, e, `SELECT count(*) FROM person WHERE id = $1 AND archived_at IS NULL`, contested); n != 1 {
		t.Fatal("the sweep retracted on a stale noise row although a later verdict called the sender real")
	}
	if n := countIn(t, e, `SELECT count(*) FROM person WHERE id = $1 AND archived_at IS NULL`, answered); n != 1 {
		t.Fatal("the sweep retracted a sender the workspace has since written to — correspondence must call the old verdict off the record")
	}
	if n := countIn(t, e, `SELECT count(*) FROM person WHERE id = $1 AND archived_at IS NULL`, readmitted); n != 1 {
		t.Fatal("the sweep retracted a contact whose owner had marked the sender business — a standing human decision lost to a machine verdict")
	}
}

// seedCaptureOnlyContact inserts a person exactly as capture minted one before
// the tiered creation gate existed: connector-made, owner-scoped, never touched
// by a human — the rows the retraction exists to reach, which today's sink
// refuses to create and so cannot seed.
func seedCaptureOnlyContact(t *testing.T, e *integration.Env, email string, owner ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO person (id, owner_id, full_name, source, captured_by, visibility)
			VALUES ($1, $2, $3, 'gmail', 'connector:gmail', 'owner')`, id, owner, email); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'gmail', 'connector:gmail')`, id, email)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the captured contact: %v", err)
	}
	return id
}

// seedSenderOverride records a seat's decision directly — the row
// SenderOverrideStore.Set writes, without the HTTP principal it requires.
func seedSenderOverride(t *testing.T, e *integration.Env, user ids.UUID, address, decision string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_sender_override (user_id, address, decision)
			VALUES ($1, $2, $3)`, user, address, decision)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the sender decision: %v", err)
	}
}

// seedSettledNoiseRow writes a ledger row already resolved as noise — the state
// every sender judged before the retraction shipped is in.
func seedSettledNoiseRow(t *testing.T, e *integration.Env, email, kind string) {
	t.Helper()
	seedSettledRow(t, e, email, capture.PendingStatusNoise, kind)
}

func seedSettledRow(t *testing.T, e *integration.Env, email, status, kind string) {
	t.Helper()
	activityID := seedCapturedMail(t, e, email, "already judged")
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_pending_counterparty
			  (email, domain, activity_id, owner_id, status, kind, resolved_at)
			VALUES ($1, split_part($1, '@', 2), $2, $3, $4, $5, now())`,
			email, activityID, e.Rep1, status, kind)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the settled ledger row: %v", err)
	}
}
