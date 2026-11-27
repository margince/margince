// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedDurations stores one call per duration, each as an older writer stored
// it, before the floor refused a negative one.
func seedDurations(t *testing.T, durations ...int64) []ids.UUID {
	t.Helper()
	calls := make([]ids.UUID, len(durations))
	for i := range calls {
		calls[i] = ids.NewV7()
	}
	testdb.WithNegativeCountFloorsLifted(t, func(tx pgx.Tx) error {
		for i, call := range calls {
			if _, err := tx.Exec(t.Context(), `INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, source, captured_by)
				VALUES ($1, 'call', 'Intro call', now(), $2, 'manual', 'test')`, call, durations[i]); err != nil {
				return err
			}
		}
		return nil
	})
	return calls
}

// storedDuration reads an activity's duration, nil when it is unknown.
func storedDuration(t *testing.T, e *sendEnv, call ids.UUID) *int64 {
	t.Helper()
	var duration *int64
	if err := e.owner.QueryRow(context.Background(), `SELECT duration_seconds FROM activity WHERE id = $1`, call).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	return duration
}

// clearDurations runs the data task's step to its end, one row a batch, and
// answers how many rows it changed.
func clearDurations(t *testing.T, e *sendEnv) int {
	t.Helper()
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), "system:negative-counts")
	in, changed := datatask.BatchIn{Limit: 1}, 0
	for {
		var out datatask.BatchOut
		if err := database.WithTx(system, e.pool, func(tx pgx.Tx) error {
			var err error
			out, err = ClearNegativeDurationsTx(system, tx, in)
			return err
		}); err != nil {
			t.Fatalf("clearing negative durations: %v", err)
		}
		changed += out.Changed
		if out.Done {
			return changed
		}
		in.Cursor = out.Cursor
	}
}

func TestANegativeDurationBecomesUnknownAndOtherDurationsStay(t *testing.T) {
	e := setupSend(t)
	calls := seedDurations(t, -60, -1, 0, 300)
	negative, alsoNegative, zero, measured := calls[0], calls[1], calls[2], calls[3]

	if changed := clearDurations(t, e); changed != 2 {
		t.Errorf("the task changed %d rows, want the two negative durations", changed)
	}
	for _, call := range []ids.UUID{negative, alsoNegative} {
		if d := storedDuration(t, e, call); d != nil {
			t.Errorf("a negative duration reads %d after the task, want NULL: the real duration was never known", *d)
		}
	}
	if d := storedDuration(t, e, zero); d == nil || *d != 0 {
		t.Errorf("a zero duration reads %v, want it untouched", d)
	}
	if d := storedDuration(t, e, measured); d == nil || *d != 300 {
		t.Errorf("a 300 second duration reads %v, want it untouched", d)
	}
	var before string
	if err := e.owner.QueryRow(context.Background(), `SELECT before->>'duration_seconds' FROM audit_log
		 WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update' AND after ? 'duration_seconds'
		   AND after->'duration_seconds' = 'null'::jsonb`, negative).Scan(&before); err != nil || before != "-60" {
		t.Errorf("the audit row of the cleared duration holds %q, %v; want the -60 it replaced", before, err)
	}
	if changed := clearDurations(t, e); changed != 0 {
		t.Errorf("a replay changed %d rows, want none", changed)
	}
}

// An old negative row stays editable before the task reaches it: the edit
// clears the duration it carries. A new negative duration is still refused.
func TestAnEditClearsAnOldNegativeDurationAndANewOneIsRefused(t *testing.T) {
	e := setupSend(t)
	calls := seedDurations(t, -60, 120)
	negative, measured := calls[0], calls[1]
	ctx := context.Background()

	if _, err := e.owner.Exec(ctx, `UPDATE activity SET subject = 'Renamed call' WHERE id = $1`, negative); err != nil {
		t.Fatalf("editing an activity with an old negative duration: %v", err)
	}
	if d := storedDuration(t, e, negative); d != nil {
		t.Errorf("the edited activity's duration reads %d, want NULL", *d)
	}
	_, err := e.owner.Exec(ctx, `UPDATE activity SET duration_seconds = -5 WHERE id = $1`, measured)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); !ok || pgErr.ConstraintName != "activity_duration_not_negative" {
		t.Errorf("writing a new negative duration = %v, want the floor to refuse it", err)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE activity SET duration_seconds = -5 WHERE id = $1`, negative); err == nil {
		t.Error("writing a negative duration over a cleared one succeeded, want the floor to refuse it")
	}
}

// A restricted row is immutable, so the task passes it and finishes. The
// erasure that lifts its restriction clears the duration in the same update.
func TestARestrictedNegativeDurationWaitsForItsLift(t *testing.T) {
	e := setupSend(t)
	held := ids.NewV7()
	testdb.WithNegativeCountFloorsLifted(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, source, captured_by,
			  retention_class, retention_class_at, archived_at, restricted_at, restricted_reason, restricted_until)
			VALUES ($1, 'call', 'Contract call', now(), -60, 'manual', 'test',
			  'commercial_correspondence', now(), now(), now(), 'statutory retention', now() + interval '6 years')`, held)
		return err
	})
	measured := seedDurations(t, -30)[0]

	if changed := clearDurations(t, e); changed != 1 {
		t.Errorf("the task changed %d rows, want only the unrestricted one", changed)
	}
	if d := storedDuration(t, e, held); d == nil || *d != -60 {
		t.Errorf("the restricted row's duration reads %v, want -60 left as the restriction holds it", d)
	}
	if d := storedDuration(t, e, measured); d != nil {
		t.Errorf("the unrestricted row on the same run reads %d, want NULL", *d)
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE activity SET restricted_at = NULL, restricted_reason = NULL,
		  restricted_until = NULL, body = NULL, raw = NULL, counterparty_email = NULL, source_author_name = NULL,
		  subject = 'Erased' WHERE id = $1`, held); err != nil {
		t.Fatalf("lifting the restriction: %v", err)
	}
	if d := storedDuration(t, e, held); d != nil {
		t.Errorf("the lifted row's duration reads %d, want NULL", *d)
	}
}
