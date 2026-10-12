// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the noise redaction sweep must leave standing, and what one message it
// cannot finish must not cost the others.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// hiddenNoiseMail seeds one bulk message per sender, runs the verdict pass
// that hides them all, and ages each past its undo window.
func hiddenNoiseMail(t *testing.T, e *integration.Env, senders ...string) (*CounterpartyVerdictEngine, map[string]ids.UUID) {
	t.Helper()
	verdicts := map[string]string{}
	mail := map[string]ids.UUID{}
	for _, sender := range senders {
		id := seedBulkCapturedMail(t, e, sender, "offer")
		disposition := seedPendingDisposition(t, e, sender, "bulk.example", id)
		verdicts[disposition.String()] = capture.KindSpam
		mail[sender] = id
	}
	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{verdicts: verdicts}, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	for _, id := range mail {
		backdateArchive(t, e, id)
	}
	return engine, mail
}

func redactNoise(t *testing.T, e *integration.Env, engine *CounterpartyVerdictEngine, maxRows int) {
	t.Helper()
	if err := engine.RedactNoiseWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), capture.NoiseUndoWindow, maxRows); err != nil {
		t.Fatalf("redaction sweep: %v", err)
	}
}

func hasContent(t *testing.T, e *integration.Env, id ids.UUID) bool {
	t.Helper()
	return countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NOT NULL`, id) == 1
}

// A legal hold, an open subject request and a colleague's import each keep a
// message out of every destructive path. The noise sweep is one.
func TestTheNoiseSweepLeavesProtectedMailStanding(t *testing.T) {
	e := integration.Setup(t)
	const held, requested, shared, plain = "held@bulk.example", "asked@bulk.example", "shared@bulk.example", "plain@bulk.example"
	engine, mail := hiddenNoiseMail(t, e, held, requested, shared, plain)

	company := e.SeedCompany(t, "Litigation Co", nil)
	e.WsExec(t, `UPDATE company SET legal_hold = true WHERE id = $1`, company)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id)
		VALUES ($1, 'company', $2)`, mail[held], company)

	subject := e.SeedContact(t, "Asking Subject", nil)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by, from_correspondence)
		VALUES ($1, $2, true, 'test', 'human:seed', false)`, subject, requested)
	e.WsExec(t, `INSERT INTO data_subject_request (id, kind, status, subject_ref, contact_id, due_at)
		VALUES ($1, 'access', 'open', $2::text, $2::uuid, now() + interval '30 days')`, ids.NewV7(), subject)

	for _, seat := range []ids.UUID{e.Rep1, e.AdminUser} {
		e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id, posture_at_import)
			VALUES ($1, $2, 'classified')`, mail[shared], seat)
	}

	redactNoise(t, e, engine, 0)

	if hasContent(t, e, mail[plain]) {
		t.Fatal("the sweep did not redact the unprotected message, so the rest of this test proves nothing")
	}
	for sender, why := range map[string]string{
		held: "a legal hold on a linked company", requested: "an open subject request", shared: "a colleague's import",
	} {
		if !hasContent(t, e, mail[sender]) {
			t.Errorf("the sweep destroyed noise mail protected by %s", why)
		}
	}
}

// The Message-ID and the reply links stay with the redacted row. They are the
// tombstone that ties a later arrival of the same message to what was judged.
func TestRedactedNoiseMailKeepsItsMessageIdentity(t *testing.T) {
	e := integration.Setup(t)
	engine, mail := hiddenNoiseMail(t, e, "loud@bulk.example")
	noise := mail["loud@bulk.example"]
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		activity := ids.From[ids.ActivityKind](noise)
		if _, err := activities.ClaimIdentity(ctx, tx, activity, activities.IdentityKindMail,
			"<offer-1@bulk.example>", "connector:gmail"); err != nil {
			return err
		}
		return activities.RecordMailReferencesTx(ctx, tx, activity, []string{"<offer-0@bulk.example>"})
	})
	if err != nil {
		t.Fatalf("seeding the message identity: %v", err)
	}

	redactNoise(t, e, engine, 0)

	if hasContent(t, e, noise) {
		t.Fatal("the sweep did not redact the message, so this test proves nothing")
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity_identity WHERE activity_id = $1`, noise); n != 1 {
		t.Error("the redaction dropped the Message-ID the tombstone answers to")
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity_mail_reference WHERE activity_id = $1`, noise); n != 1 {
		t.Error("the redaction dropped the reply links of the tombstone")
	}
}

// A message the sweep cannot finish rolls back alone. The oldest message here
// carries attachment bytes and the engine has no object store, so it fails on
// every pass. A page of one proves the sweep walks past it; the default page
// proves the two messages do not share a transaction.
func TestOneNoiseMessageThatFailsDoesNotHoldBackTheRest(t *testing.T) {
	for _, pageSize := range []int{1, 0} {
		e := integration.Setup(t)
		engine, mail := hiddenNoiseMail(t, e, "stuck@bulk.example", "next@bulk.example")
		stuck, next := mail["stuck@bulk.example"], mail["next@bulk.example"]
		e.WsExec(t, `UPDATE activity SET occurred_at = occurred_at - interval '1 day' WHERE id = $1`, stuck)
		e.WsExec(t, `
			INSERT INTO attachment (id, entity_type, entity_id, filename, byte_size, storage_key, checksum, source, captured_by)
			VALUES ($1, 'activity', $2, 'brochure.pdf', 12, $3, 'sum', 'capture', 'connector:gmail')`,
			ids.NewV7(), stuck, "attachments/"+stuck.String())

		redactNoise(t, e, engine, pageSize)

		if hasContent(t, e, next) {
			t.Errorf("page size %d: one message the sweep could not finish kept the next one from being redacted", pageSize)
		}
		if !hasContent(t, e, stuck) {
			t.Errorf("page size %d: the failing message lost its text while its attachment bytes could not be purged", pageSize)
		}
	}
}
