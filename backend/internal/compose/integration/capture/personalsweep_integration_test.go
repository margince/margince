// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The personal sweep spends its batch on mail it may destroy. Mail a shield
// keeps sits in front of the eligible mail by age. A seat with a full batch of
// it must still reach what lies behind. What the shields kept is counted over
// the whole seat, so the receipt does not depend on what one batch reached.

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepBatch is one pass's batch per seat, which the held mail must exceed.
const sweepBatch = 500

// seedImportedMail files one received message the seat imported.
func seedImportedMail(t *testing.T, e *integration.Env, from, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, audience)
			VALUES ($1, 'email', $2, 'the message body', 'inbound', 'gmail', $3,
			        'gmail:'||$3, 'connector:gmail', $4, 'participants')`,
			id, subject, "pg-"+id.String(), from); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import)
			VALUES ($1, $2, 'classified')`, id, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding imported mail: %v", err)
	}
	return id
}

// resolveAsPersonal records the seat's own verdict that the sender is personal,
// through the ledger's writer.
func resolveAsPersonal(t *testing.T, e *integration.Env, from string, activity ids.UUID) {
	t.Helper()
	store := capturemod.NewPendingStore(compose.InstallationDB(e.Pool))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_pending_counterparty
			       (email, domain, activity_id, owner_id, status, next_attempt_at)
			VALUES ($1, split_part($1, '@', 2), $2, $3, 'pending', now())`, from, activity, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("seeding the pending sender: %v", err)
	}
	due, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming the pending sender: %v", err)
	}
	for _, row := range due {
		if row.Email != from {
			continue
		}
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			won, err := store.ResolveAs(e.Admin(), tx, row, capturemod.PendingStatusNoise, capturemod.KindPersonal,
				"test", true, capturemod.VerdictMeasurement{})
			if err == nil && !won {
				t.Fatal("the resolve did not win its own claim")
			}
			return err
		}); err != nil {
			t.Fatalf("resolving the sender as personal: %v", err)
		}
		return
	}
	t.Fatalf("the ledger handed out no row for %s", from)
}

// seedDueMail files n messages from one sender the seat marked personal, all
// past the owner window, and returns the first.
func seedDueMail(t *testing.T, e *integration.Env, address string, n int) ids.UUID {
	t.Helper()
	first := seedImportedMail(t, e, address, "Nachricht 0")
	resolveAsPersonal(t, e, address, first)
	for i := 1; i < n; i++ {
		seedImportedMail(t, e, address, fmt.Sprintf("Nachricht %d", i))
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`UPDATE activity SET created_at = now() - interval '8 days' WHERE counterparty_email = $1`, address); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_pending_counterparty SET resolved_at = now() - interval '8 days'
			  WHERE email = $1 AND resolved_at IS NOT NULL`, address)
		return err
	}); err != nil {
		t.Fatalf("aging the mail of %s: %v", address, err)
	}
	return first
}

func keepsItsBody(t *testing.T, e *integration.Env, address string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity WHERE counterparty_email = $1 AND body = 'the message body'`, address)
}

// sweepOnce runs one sweep pass the way the worker does and returns how many
// messages it destroyed.
func sweepOnce(t *testing.T, e *integration.Env) int {
	t.Helper()
	purger := compose.NewCapturePurger(e.Pool,
		compose.NewRetentionServiceFor(compose.InstallationDB(e.Pool), nil, slog.Default()))
	n, err := purger.SweepPersonalMail(principal.WithWorkspaceID(context.Background(), e.WS),
		capturemod.DefaultPersonalPurgeWindows())
	if err != nil {
		t.Fatalf("sweeping personal mail: %v", err)
	}
	return n
}

// assertReceipt checks that the one audit row of the pass names the numbers.
func assertReceipt(t *testing.T, e *integration.Env, kind string, kept int) {
	t.Helper()
	matching := e.WsCount(t, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'capture_personal_sweep' AND entity_id = $1
		   AND (evidence->>'destroyed')::int = 1
		   AND (evidence->>'skipped')::int = $2
		   AND (evidence->>$3)::int = $2`, e.Rep1, kept, kind)
	if matching != 1 {
		t.Fatalf("audit rows naming one destroyed and %d %s = %d, want exactly 1", kept, kind, matching)
	}
}

func TestEligibleMailBehindAFullBatchOfMailAnOpenRequestKeepsIsDestroyedInOnePass(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	const held = "anwalt@kanzlei.example"
	heldCount := sweepBatch + 1
	seedDueMail(t, e, held, heldCount)
	subject := e.SeedContact(t, "Der Mandant", nil)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, $3, 'work', true, 'manual', 'human:x')`, ids.NewV7(), subject, held); err != nil {
		t.Fatalf("seeding the subject's address: %v", err)
	}
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO data_subject_request (id, kind, status, subject_ref, contact_id, due_at)
		VALUES ($1, 'erasure', 'open', $2::text, $2::uuid, now() + interval '30 days')`, ids.NewV7(), subject); err != nil {
		t.Fatalf("seeding the open request: %v", err)
	}
	eligible := seedDueMail(t, e, "mama@familie.example", 1)

	if n := sweepOnce(t, e); n != 1 {
		t.Fatalf("destroyed %d messages, want the one eligible message behind %d held ones", n, heldCount)
	}
	if kept := keepsItsBody(t, e, "mama@familie.example"); kept != 0 {
		t.Fatalf("the eligible message %s kept its body behind a full batch of held mail", eligible)
	}
	if kept := keepsItsBody(t, e, held); kept != heldCount {
		t.Fatalf("%d of %d held messages kept their body, want all of them", kept, heldCount)
	}
	assertReceipt(t, e, "kept_under_request", heldCount)
}

func TestEligibleMailBehindAFullBatchUnderTheStatutoryFloorIsDestroyedInOnePass(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	const held = "einkauf@kunde.example"
	heldCount := sweepBatch + 1
	first := seedDueMail(t, e, held, heldCount)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity
			   SET retention_class = 'commercial_correspondence', retention_class_at = now(),
			       occurred_at = now() - interval '30 days'
			 WHERE counterparty_email = $1`, held)
		return err
	}); err != nil {
		t.Fatalf("stamping commercial correspondence: %v", err)
	}
	// The floor is a compiled-in pack and the default build declares none, and
	// then this mail is destroyable and there is no shield to test.
	var open bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		policy, err := privacy.ResolveFloorPolicy(context.Background(), tx, identity.CountryOf)
		if err != nil {
			return err
		}
		current, carried := policy.ShieldArgs()
		return tx.QueryRow(context.Background(),
			`SELECT (`+privacy.StatutoryFloorShield(2, 3)+`) FROM activity a WHERE a.id = $1`,
			first, current, carried).Scan(&open)
	}); err != nil {
		t.Fatalf("asking whether the statutory window is open: %v", err)
	}
	if !open {
		return
	}
	seedDueMail(t, e, "mama@familie.example", 1)

	if n := sweepOnce(t, e); n != 1 {
		t.Fatalf("destroyed %d messages, want the one eligible message behind %d statutory-floor ones", n, heldCount)
	}
	assertReceipt(t, e, "kept_under_statute", heldCount)
}

func TestAHeldMessageIsKeptWhileTheMailBehindItIsDestroyed(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	const held = "gegner@kanzlei.example"
	heldMail := seedDueMail(t, e, held, 1)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_retention_evidence
			  (activity_id, basis, decided_by_name, reason, qualified_at)
			VALUES ($1, 'controller_pin', 'Test Controller', 'statutory retention under test', now())`,
			heldMail); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			UPDATE activity
			   SET retention_class = 'commercial_correspondence', retention_class_at = now(),
			       archived_at = coalesce(archived_at, now()), restricted_at = now(),
			       restricted_reason = 'statutory retention', restricted_until = now() + interval '6 years'
			 WHERE id = $1`, heldMail)
		return err
	}); err != nil {
		t.Fatalf("holding the message: %v", err)
	}
	seedDueMail(t, e, "mama@familie.example", 1)

	if n := sweepOnce(t, e); n != 1 {
		t.Fatalf("destroyed %d messages, want only the one that is not held", n)
	}
	if kept := keepsItsBody(t, e, held); kept != 1 {
		t.Fatal("the held message lost its body to the sweep")
	}
}
