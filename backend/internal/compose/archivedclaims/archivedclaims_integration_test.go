// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package archivedclaims_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/archivedclaims"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// claimPair plants a kept claim and the duplicate the evidence index archived
// beside it, the state the index migration leaves.
func claimPair(t *testing.T, e *integration.Env, subject string) (kept, archived ids.UUID) {
	t.Helper()
	contact, activity := ids.NewV7(), ids.NewV7()
	kept, archived = ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:test')`, contact, subject)
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at)
		VALUES ($1, 'email', $2, 'manual', 'human:test', now())`, activity, subject)
	e.WsExec(t, `INSERT INTO conversation_claim
		(id, contact_id, kind, body, source_activity_id, source_quote, evidence_fingerprint, source, captured_by, status, archived_at)
		VALUES ($1, $3, 'commitment_ours', 'send it', $4, 'we will send it', 'quote', 'manual', 'human:test', 'done', NULL),
		       ($2, $3, 'commitment_ours', 'send it', $4, 'we will send it', 'quote', 'manual', 'human:test', 'open', now())`,
		kept, archived, contact, activity)
	return kept, archived
}

func newEngine(t *testing.T, e *integration.Env, task datatask.Task, at time.Time) *datatask.Engine {
	t.Helper()
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return at },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestTheArchivedDuplicateClaimsAreDeletedThroughTheExecutor(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:test")
	keptA, archivedA := claimPair(t, e, "First customer")
	keptB, archivedB := claimPair(t, e, "Second customer")
	archived := []ids.UUID{archivedA, archivedB}

	task := archivedclaims.DataTask()
	task.Steps[0].Limit = 1
	if count, capped, err := task.Steps[0].Estimate(ctx, e.Pool, 10); err != nil || count != 2 || capped {
		t.Fatalf("the estimate reads %d (capped %v, err %v), want the two archived claims", count, capped, err)
	}
	original := task.Steps[0].Batch
	rollback := errors.New("rollback after write")
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, rollback
	}
	now := clocktest.Now(t)
	engine := newEngine(t, e, task, now)
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE id = ANY($1)`, archived); n != 2 {
		t.Fatalf("a failed batch committed a delete: %d of 2 archived claims remain", n)
	}

	task.Steps[0].Batch = original
	engine = newEngine(t, e, task, now.Add(time.Hour))
	for range 10 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE id = ANY($1)`, archived); n != 0 {
		t.Fatalf("%d archived claims remain", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE id = ANY($1)`, []ids.UUID{keptA, keptB}); n != 2 {
		t.Fatalf("the task deleted a kept claim: %d of 2 remain", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact'
		AND after->>'claim_id' = ANY($1::text[]) AND (after->>'claim_erased')::boolean`, uuidTexts(archived)); n != 2 {
		t.Fatalf("%d audit rows record the deletes, want one per archived claim", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'conversation_claim.changed'
		AND envelope->'payload'->>'claim_id' = ANY($1::text[]) AND envelope->'payload'->>'status' = 'erased'`, uuidTexts(archived)); n != 2 {
		t.Fatalf("%d events announce the deletes, want one per archived claim", n)
	}
	if count, _, err := task.Steps[0].Estimate(ctx, e.Pool, 10); err != nil || count != 0 {
		t.Fatalf("the estimate reads %d after the task (err %v), want none left", count, err)
	}
}

func TestADeletedClaimReplayWritesNothingAgain(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:test")
	_, archived := claimPair(t, e, "Replayed customer")
	task := archivedclaims.DataTask()
	engine := newEngine(t, e, task, clocktest.Now(t))
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	var replay datatask.BatchOut
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
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
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE after->>'claim_id' = $1`, archived.String()); n != 1 {
		t.Fatalf("%d audit rows name the deleted claim after a replay, want 1", n)
	}
	if _, err := task.Steps[0].Batch(ctx, nil, datatask.BatchIn{Cursor: []byte(`"invalid"`), Limit: 1}); err == nil {
		t.Fatal("a malformed cursor was accepted")
	}
}

func uuidTexts(list []ids.UUID) []string {
	out := make([]string, len(list))
	for i, id := range list {
		out[i] = id.String()
	}
	return out
}
