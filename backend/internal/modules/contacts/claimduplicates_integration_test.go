// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedArchivedClaimPair plants a kept claim and the duplicate the evidence
// index archived beside it, the state the index migration leaves.
func seedArchivedClaimPair(ctx context.Context, t *testing.T, e *dedupeEnv, subject string) (kept, archived ids.UUID) {
	t.Helper()
	contact, activity := ids.NewV7(), ids.NewV7()
	kept, archived = ids.NewV7(), ids.NewV7()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:test')`,
			contact, subject); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at)
			VALUES ($1, 'email', $2, 'manual', 'human:test', now())`, activity, subject); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO conversation_claim
			(id, contact_id, kind, body, source_activity_id, source_quote, evidence_fingerprint, source, captured_by, status, archived_at)
			VALUES ($1, $3, 'commitment_ours', 'send it', $4, 'we will send it', 'quote', 'manual', 'human:test', 'done', NULL),
			       ($2, $3, 'commitment_ours', 'send it', $4, 'we will send it', 'quote', 'manual', 'human:test', 'open', now())`,
			kept, archived, contact, activity)
		return err
	}); err != nil {
		t.Fatalf("seed a claim pair: %v", err)
	}
	return kept, archived
}

func (e *dedupeEnv) countClaimRows(ctx context.Context, t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error { return tx.QueryRow(ctx, sql, args...).Scan(&n) }); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func archivedClaimsEngine(t *testing.T, e *dedupeEnv, task datatask.Task, at time.Time) *datatask.Engine {
	t.Helper()
	engine, err := datatask.New(e.store.db.Pool(), []datatask.Task{task}, "test", func() time.Time { return at },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestTheArchivedDuplicateClaimsAreDeletedThroughTheExecutor(t *testing.T) {
	e := setupDedupe(t)
	ctx := principal.SystemActing(e.as(), "system:test")
	keptA, archivedA := seedArchivedClaimPair(ctx, t, e, "First customer")
	keptB, archivedB := seedArchivedClaimPair(ctx, t, e, "Second customer")
	archived := []string{archivedA.String(), archivedB.String()}

	task := ArchivedClaimsDataTask()
	task.Steps[0].Limit = 2
	if count, capped, err := task.Steps[0].Estimate(ctx, e.store.db.Pool(), 10); err != nil || count != 2 || capped {
		t.Fatalf("the estimate reads %d (capped %v, err %v), want the two archived claims", count, capped, err)
	}
	original := task.Steps[0].Batch
	rollback := errors.New("rollback after write")
	deletedBeforeRollback := 0
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		deletedBeforeRollback += out.Changed
		return out, rollback
	}
	now := clocktest.Now(t)
	engine := archivedClaimsEngine(t, e, task, now)
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.countClaimRows(ctx, t, `SELECT count(*) FROM conversation_claim WHERE id::text = ANY($1)`, archived); n != 2 {
		t.Fatalf("a failed batch committed a delete: %d of 2 archived claims remain", n)
	}
	if deletedBeforeRollback == 0 {
		t.Fatal("the failed batch deleted nothing, so its rollback proves nothing")
	}

	task.Steps[0].Batch = original
	task.Steps[0].Limit = 1
	engine = archivedClaimsEngine(t, e, task, now.Add(time.Hour))
	for range 10 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.countClaimRows(ctx, t, `SELECT count(*) FROM conversation_claim WHERE id::text = ANY($1)`, archived); n != 0 {
		t.Fatalf("%d archived claims remain", n)
	}
	if n := e.countClaimRows(ctx, t, `SELECT count(*) FROM conversation_claim WHERE id = ANY($1)`, []ids.UUID{keptA, keptB}); n != 2 {
		t.Fatalf("the task deleted a kept claim: %d of 2 remain", n)
	}
	if n := e.countClaimRows(ctx, t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact'
		AND after->>'claim_id' = ANY($1) AND (after->>'claim_erased')::boolean`, archived); n != 2 {
		t.Fatalf("%d audit rows record the deletes, want one per archived claim", n)
	}
	if n := e.countClaimRows(ctx, t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'conversation_claim.changed'
		AND envelope->'payload'->>'claim_id' = ANY($1) AND envelope->'payload'->>'status' = 'erased'`, archived); n != 2 {
		t.Fatalf("%d events announce the deletes, want one per archived claim", n)
	}
	if count, _, err := task.Steps[0].Estimate(ctx, e.store.db.Pool(), 10); err != nil || count != 0 {
		t.Fatalf("the estimate reads %d after the task (err %v), want none left", count, err)
	}
}

func TestADeletedClaimReplayWritesNothingAgain(t *testing.T) {
	e := setupDedupe(t)
	ctx := principal.SystemActing(e.as(), "system:test")
	_, archived := seedArchivedClaimPair(ctx, t, e, "Replayed customer")
	task := ArchivedClaimsDataTask()
	engine := archivedClaimsEngine(t, e, task, clocktest.Now(t))
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	var replay datatask.BatchOut
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		replay, err = task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if replay.Changed != 0 || replay.Scanned == 0 {
		t.Fatalf("a replay from the first row inspected %d and changed %d, want rows inspected and none changed",
			replay.Scanned, replay.Changed)
	}
	if n := e.countClaimRows(ctx, t, `SELECT count(*) FROM audit_log WHERE after->>'claim_id' = $1`, archived.String()); n != 1 {
		t.Fatalf("%d audit rows name the deleted claim after a replay, want 1", n)
	}
}
