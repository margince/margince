// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The personal sweep spends its batch on mail it may destroy. Mail a shield
// keeps sits in front of the eligible mail by age, and a seat with a full batch
// of it must still reach what lies behind. What the shields kept is counted over
// the whole seat, so the receipt does not depend on what one batch reached.

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedDuePersonalMail files n messages from one sender the seat marked personal,
// all older than the owner window, and returns the first.
func seedDuePersonalMail(t *testing.T, e *integration.Env, address string, n int) ids.UUID {
	t.Helper()
	first := seedPurgeableMail(t, e, address, "Nachricht 0", e.Rep1)
	resolvePersonal(t, e, address, first, e.Rep1, true)
	for i := 1; i < n; i++ {
		seedPurgeableMail(t, e, address, fmt.Sprintf("Nachricht %d", i), e.Rep1)
	}
	agePersonalMail(t, e, first, address, 8*24*time.Hour)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET created_at = now() - interval '8 days' WHERE counterparty_email = $1`, address)
		return err
	}); err != nil {
		t.Fatalf("aging the mail of %s: %v", address, err)
	}
	return first
}

// sweepOneSeat runs one sweep pass for the seat the way the worker does, and
// returns the receipt it wrote.
func sweepOneSeat(t *testing.T, e *integration.Env) PurgeOutcome {
	t.Helper()
	purger := NewCapturePurger(e.Pool, NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.Default()))
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), personalSweepActor)
	outcome, err := purger.sweepSeat(ctx, e.Rep1, capture.DefaultPersonalPurgeWindows())
	if err != nil {
		t.Fatalf("sweeping one seat: %v", err)
	}
	return outcome
}

// assertSweepReceiptAgrees checks that the audit row of the pass carries the
// numbers of the receipt it was written from.
func assertSweepReceiptAgrees(t *testing.T, e *integration.Env, outcome PurgeOutcome) {
	t.Helper()
	matching := e.WsCount(t, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'capture_personal_sweep' AND entity_id = $1
		   AND (evidence->>'destroyed')::int = $2
		   AND (evidence->>'skipped')::int = $3
		   AND (evidence->>'kept_under_statute')::int = $4
		   AND (evidence->>'kept_under_request')::int = $5`,
		e.Rep1, outcome.Destroyed, outcome.Skipped, outcome.Kept.UnderStatute, outcome.Kept.UnderRequest)
	if matching != 1 {
		t.Fatalf("audit rows agreeing with the receipt = %d, want exactly 1 for %+v", matching, outcome)
	}
}

func TestEligibleMailBehindAFullBatchOfMailAnOpenRequestKeepsIsDestroyedInOnePass(t *testing.T) {
	e := integration.Setup(t)
	const held = "anwalt@kanzlei.example"
	heldCount := personalSweepBatch + 1
	seedDuePersonalMail(t, e, held, heldCount)
	seedRequestFor(t, e, held, "open", "")
	eligible := seedDuePersonalMail(t, e, "mama@familie.example", 1)

	outcome := sweepOneSeat(t, e)

	if outcome.Destroyed != 1 {
		t.Fatalf("destroyed %d messages, want the one eligible message behind %d held ones", outcome.Destroyed, heldCount)
	}
	if body := activityBody(t, e, eligible); body != "" {
		t.Fatalf("the eligible message kept its body %q behind a full batch of held mail", body)
	}
	if kept := e.WsCount(t, `SELECT count(*) FROM activity WHERE counterparty_email = $1 AND body = 'the message body'`, held); kept != heldCount {
		t.Fatalf("%d of %d held messages kept their body, want all of them", kept, heldCount)
	}
	if outcome.Kept.UnderRequest != heldCount {
		t.Fatalf("kept.under_request = %d, want every held message counted, not the batch's share (%d)",
			outcome.Kept.UnderRequest, heldCount)
	}
	assertSweepReceiptAgrees(t, e, outcome)
}

func TestEligibleMailBehindAFullBatchUnderTheStatutoryFloorIsDestroyedInOnePass(t *testing.T) {
	e := integration.Setup(t)
	const held = "einkauf@kunde.example"
	heldCount := personalSweepBatch + 1
	first := seedDuePersonalMail(t, e, held, heldCount)
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
	if !statutoryWindowIsOpen(t, e, first) {
		return
	}
	eligible := seedDuePersonalMail(t, e, "mama@familie.example", 1)

	outcome := sweepOneSeat(t, e)

	if outcome.Destroyed != 1 || activityBody(t, e, eligible) != "" {
		t.Fatalf("the eligible message was not destroyed behind %d statutory-floor messages: %+v", heldCount, outcome)
	}
	if outcome.Kept.UnderStatute != heldCount {
		t.Fatalf("kept.under_statute = %d, want every shielded message counted (%d)", outcome.Kept.UnderStatute, heldCount)
	}
	assertSweepReceiptAgrees(t, e, outcome)
}
