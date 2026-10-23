// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const guessedSeatMailbox = "rep1@myco.example"

// seedGuessedSeat gives a bare-stamped legacy mail the two rows the participant
// backfill wrote when it read the provider's only connection as the owner: the
// seat, and the counterparty's address. No writer produces that shape any more.
func seedGuessedSeat(t *testing.T, e *integration.Env, conn *pgx.Conn, sourceID string, seat ids.UUID) ids.UUID {
	t.Helper()
	id := seedLegacyActivity(t, e, sourceID, "connector:gmail", "inbound", "pat@counterparty.test")
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'to')`, id, seat); err != nil {
		t.Fatalf("seeding the guessed owner of %s: %v", sourceID, err)
	}
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'pat@counterparty.test', 'from')`, id); err != nil {
		t.Fatalf("seeding the counterparty of %s: %v", sourceID, err)
	}
	return id
}

func ownSeatRows(t *testing.T, e *integration.Env, activity ids.UUID, seat ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_participant
		WHERE activity_id = $1 AND user_id = $2 AND address IS NULL AND contact_id IS NULL`, activity, seat)
}

func runGuessedMailboxTask(ctx context.Context, t *testing.T, e *integration.Env, task datatask.Task) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func guessedMailboxPreview(ctx context.Context, t *testing.T, e *integration.Env) int64 {
	t.Helper()
	count, _, err := guessedMailboxSeatsDataTask().Steps[0].Estimate(ctx, e.Pool, 100)
	if err != nil {
		t.Fatalf("previewing the repair: %v", err)
	}
	return count
}

// The repair takes off only the owner nothing proves. A seat whose mailbox the
// capture ledger, the capturing connector or a proven address ties to the mail
// keeps it, as does mail a colleague has acted on and mail stamped with a seat.
func TestTheGuessedMailboxRepairRemovesOnlyUnprovenOwners(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if _, err := conn.Exec(ctx, `INSERT INTO capture_connection (provider, user_id, status, credential_ref, account_label)
		VALUES ('gmail', $1, 'connected', 'vault:test', $2)`, e.Rep1, guessedSeatMailbox); err != nil {
		t.Fatal(err)
	}
	guessed := seedGuessedSeat(t, e, conn, "gm-guessed", e.Rep1)
	// A colleague bound from the message's own To line is header evidence, not
	// the guessed owner, and stays.
	if _, err := conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, user_id, address, role)
		VALUES ($1, $2, 'rep2@myco.example', 'to')`, guessed, e.Rep2); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO raw_capture (source_system, source_id, payload)
		VALUES ('gmail', 'gm-guessed', '{"headers":"…"}'::jsonb)`); err != nil {
		t.Fatal(err)
	}

	ledger := seedGuessedSeat(t, e, conn, "gm-ledger", e.Rep1)
	if _, err := conn.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, ledger, e.Rep1); err != nil {
		t.Fatal(err)
	}
	// Mail capture landed for Rep1's mailbox while stamps still named only the
	// connector: the real sink's rows, restamped bare, with no ledger row left.
	creator := captureMail(t, e, e.Rep1, "gm-creator", connector.DirectionInbound, "pat@counterparty.test")
	if _, err := conn.Exec(ctx, `UPDATE activity SET captured_by = 'connector:gmail' WHERE id = $1`, creator); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DELETE FROM capture_import WHERE activity_id = $1`, creator); err != nil {
		t.Fatal(err)
	}
	addressed := seedGuessedSeat(t, e, conn, "gm-addressed", e.Rep1)
	if _, err := conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'to')`,
		addressed, guessedSeatMailbox); err != nil {
		t.Fatal(err)
	}
	worked := seedGuessedSeat(t, e, conn, "gm-worked", e.Rep2)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.AuditEvent(e.Admin(), tx, "update", "activity", worked, map[string]any{"note": "kept"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	stamped := captureMail(t, e, e.Rep2, "gm-stamped", connector.DirectionInbound, "pat@counterparty.test")

	if got := guessedMailboxPreview(ctx, t, e); got != 1 {
		t.Errorf("the preview counts %d messages, want only the one with an unproven owner", got)
	}
	task := guessedMailboxSeatsDataTask()
	task.Steps[0].Limit = 1
	runGuessedMailboxTask(ctx, t, e, task)

	if got := ownSeatRows(t, e, guessed, e.Rep1); got != 0 {
		t.Errorf("the guessed owner is still on the mail (%d rows)", got)
	}
	for name, kept := range map[string]struct{ activity, seat ids.UUID }{
		"ledger": {ledger, e.Rep1}, "creator": {creator, e.Rep1}, "address": {addressed, e.Rep1},
		"human": {worked, e.Rep2}, "seat stamp": {stamped, e.Rep2},
	} {
		if got := ownSeatRows(t, e, kept.activity, kept.seat); got != 1 {
			t.Errorf("the %s-proven owner was removed: %d rows, want 1", name, got)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address IS NOT NULL`, guessed); got != 2 {
		t.Errorf("the message's header rows went with the guessed owner: %d rows, want the counterparty and the colleague", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity a JOIN raw_capture rc ON rc.source_id = a.source_id WHERE a.id = $1 AND a.archived_at IS NULL`, guessed); got != 1 {
		t.Errorf("the repaired message or its stored original is gone")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
		AND before ? 'mailbox_seats' AND evidence ? 'data_task_run'`, guessed); got != 1 {
		t.Errorf("repair audit rows = %d, want one naming the run and the removed seat", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = $1`, guessed.String()); got != 1 {
		t.Errorf("activity.updated events for the repaired mail = %d, want 1", got)
	}

	if got := guessedMailboxPreview(ctx, t, e); got != 0 {
		t.Errorf("after the repair the preview still counts %d messages", got)
	}
	step := task.Steps[0]
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error { var err error; out, err = step.Batch(ctx, tx, in); return err })
		if err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay removed %d more owners", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
}

func TestTheGuessedMailboxRepairRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	guessed := seedGuessedSeat(t, e, conn, "gm-rollback", e.Rep1)

	task := guessedMailboxSeatsDataTask()
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runGuessedMailboxTask(ctx, t, e, task)

	if got := ownSeatRows(t, e, guessed, e.Rep1); got != 1 {
		t.Errorf("the owner removal survived a rolled-back batch")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, guessed); got != 0 {
		t.Errorf("repair audit rows escaped the rollback: %d", got)
	}
}
