// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedOpenedUnderVerdict lands n messages on one of Rep1's threads, opened as
// rowStatus, under a ledger that says ledgerStatus. That is the state the
// verdict path left before a closing verdict closed opened messages.
func seedOpenedUnderVerdict(t *testing.T, e *integration.Env, threadKey, ledgerStatus, rowStatus string, n int) []ids.UUID {
	t.Helper()
	var messages []ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			WITH made AS (
			  INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                        source, captured_by, counterparty_email, thread_key,
			                        audience, audience_reason)
			  SELECT gen_random_uuid(), 'email', 'Nachricht ' || n, 'the message body', 'inbound', 'gmail',
			         $1 || '-' || n, 'gmail:' || $1 || '-' || n, 'connector:gmail', 'pat@counterparty.example',
			         $1, 'participants', 'pending_verdict'
			    FROM generate_series(1, $2) AS n
			  RETURNING id)
			INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status)
			SELECT id, $3, 'classified', $4 FROM made
			RETURNING activity_id`, threadKey, n, e.Rep1, rowStatus)
		if err != nil {
			return err
		}
		messages, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		t.Fatalf("seeding opened messages: %v", err)
	}
	question := seedThreadQuestion(t, e, threadKey, messages[0])
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			UPDATE capture_thread_verdict
			   SET status = $2, resolved_at = now(), next_attempt_at = NULL
			 WHERE id = $1`, question, ledgerStatus); err != nil {
			return err
		}
		// Recreate legacy persisted visibility; the current writer already honors the ledger.
		_, err := tx.Exec(context.Background(), "UPDATE activity SET audience='workspace', audience_reason=NULL WHERE id=ANY($1)", messages)
		return err
	}); err != nil {
		t.Fatalf("settling the thread: %v", err)
	}
	return messages
}

func estimateThreadHolds(ctx context.Context, t *testing.T, e *integration.Env, task datatask.Task) int64 {
	t.Helper()
	var estimated int64
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		estimated, _, err = task.Steps[0].Estimate(ctx, tx, 10_000)
		return err
	}); err != nil {
		t.Fatalf("estimating: %v", err)
	}
	return estimated
}

func TestTheThreadHoldTaskClosesOpenedMessagesOnceAndOnlyUnderAClosingVerdict(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	held := seedOpenedUnderVerdict(t, e, "task-held", capture.VerdictHeld, capture.VerdictCleared, 2)
	unsure := seedOpenedUnderVerdict(t, e, "task-unsure", capture.VerdictUnsure, capture.VerdictSharedByOwner, 1)
	ownerHeld := seedOpenedUnderVerdict(t, e, "task-owner-held", capture.VerdictHeldByOwner, capture.VerdictCleared, 1)
	stillOpen := seedOpenedUnderVerdict(t, e, "task-cleared", capture.VerdictCleared, capture.VerdictCleared, 1)
	// A colleague's own clearance of a held thread's message is theirs.
	addImportRowFor(t, e, held[0], e.Rep2)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_import SET verdict_status = 'cleared' WHERE activity_id = $1 AND user_id = $2`,
			held[0], e.Rep2)
		return err
	}); err != nil {
		t.Fatalf("clearing the colleague's contribution: %v", err)
	}
	for _, id := range append(append(append(held, unsure...), ownerHeld...), stillOpen...) {
		if got := activityAudience(t, e, id); got != "workspace" {
			t.Fatalf("a seeded opened message is %q, want workspace", got)
		}
	}

	task := threadHoldClosesDataTask(e.Pool)
	if got := estimateThreadHolds(ctx, t, e, task); got != 4 {
		t.Fatalf("estimate = %d, want the 4 rows open under a closing verdict", got)
	}

	runMachineAddressTask(ctx, t, e, task)

	if got := estimateThreadHolds(ctx, t, e, task); got != 0 {
		t.Fatalf("estimate after the run = %d, want 0", got)
	}
	closed := map[string][]ids.UUID{
		capture.VerdictHeld: held, capture.VerdictUnsure: unsure, capture.VerdictHeldByOwner: ownerHeld,
	}
	for status, messages := range closed {
		for _, id := range messages {
			if got := activityAudience(t, e, id); got != "participants" {
				t.Errorf("a message of a thread held as %s is %q, want participants", status, got)
			}
			if got := importVerdictFor(t, e, id, e.Rep1); got != status {
				t.Errorf("its import row says %q, want %s", got, status)
			}
		}
	}
	if got := activityAudience(t, e, stillOpen[0]); got != "workspace" {
		t.Fatalf("a message of a cleared thread is %q, want workspace", got)
	}
	if got := importVerdictFor(t, e, held[0], e.Rep2); got != capture.VerdictCleared {
		t.Fatalf("the colleague's own contribution says %q, want cleared", got)
	}
	audits := func() int {
		return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = ANY($1)`, held)
	}
	before := audits()
	if before == 0 {
		t.Fatal("no audit row records the closed messages")
	}

	runMachineAddressTask(ctx, t, e, task)
	if after := audits(); after != before {
		t.Fatalf("a second run raised the audit rows from %d to %d", before, after)
	}
}

// One RecordOutcomeOnThreadTx call takes 500 rows, so a longer thread needs a
// second batch for the rest.
func TestALongHeldThreadClosesOverSeveralBatches(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	messages := seedOpenedUnderVerdict(t, e, "task-long", capture.VerdictHeld, capture.VerdictCleared, 620)

	task := threadHoldClosesDataTask(e.Pool)
	if got := estimateThreadHolds(ctx, t, e, task); got != int64(len(messages)) {
		t.Fatalf("estimate = %d, want %d", got, len(messages))
	}
	runMachineAddressTask(ctx, t, e, task)

	if got := estimateThreadHolds(ctx, t, e, task); got != 0 {
		t.Fatalf("estimate after the run = %d, want 0", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = ANY($1) AND audience <> 'participants'`, messages); n != 0 {
		t.Fatalf("%d messages of the long thread are still open", n)
	}
	if batches := e.WsCount(t, `SELECT batches FROM data_task_run WHERE task_id = $1`, task.ID); batches < 2 {
		t.Fatalf("the long thread closed in %d batch, want more than one", batches)
	}
}
