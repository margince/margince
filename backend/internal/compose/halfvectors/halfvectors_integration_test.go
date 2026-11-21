// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package halfvectors

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func systemCtx(e *integration.Env) context.Context {
	return principal.SystemActing(e.Admin(), "system:datatask-test")
}

// storedAtFullPrecision plants one vector the way rows were written before
// the half-precision column existed. No current writer makes this shape.
func storedAtFullPrecision(t *testing.T, e *integration.Env, entityType string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO embedding (entity_type, entity_id, chunk_ix, chunk_hash, model, embedding)
		VALUES ($1, $2, 0, 'hash', 'model@3', '[0.25,-0.5,0.75]'::vector)`, entityType, ids.NewV7())
}

func atFullPrecision(t *testing.T, e *integration.Env) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM embedding WHERE embedding IS NOT NULL`)
}

// The task moves every stored vector to half precision and keeps its value.
// A second pass finds nothing to move.
func TestTheTaskMovesEveryVectorToHalfPrecisionAndReplaysAsANoOp(t *testing.T) {
	e := integration.Setup(t)
	for _, entityType := range []string{"contact", "company", "deal", "company", "lead"} {
		storedAtFullPrecision(t, e, entityType)
	}
	ctx := systemCtx(e)
	if n, capped, err := DataTask().Steps[0].Estimate(ctx, e.Pool, 3); err != nil || n != 3 || !capped {
		t.Fatalf("the estimate under a cap of 3 = %d, capped %v (%v), want 3 and capped", n, capped, err)
	}
	e.RunDataTask(ctx, t, DataTask())

	if n := atFullPrecision(t, e); n != 0 {
		t.Errorf("%d vector(s) are still stored at full precision", n)
	}
	// The three components are exact in sixteen bits, so the value is unchanged.
	if n := e.WsCount(t, `SELECT count(*) FROM embedding
		WHERE embedding_half::vector = '[0.25,-0.5,0.75]'::vector`); n != 5 {
		t.Errorf("%d of 5 vectors kept their value across the move", n)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := DataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100})
		if err == nil && (out.Changed != 0 || !out.Done) {
			t.Errorf("a replay changed %d vector(s), done %v, want none and done", out.Changed, out.Done)
		}
		return err
	}); err != nil {
		t.Fatalf("replaying the batch: %v", err)
	}
}

// Batches of two walk the table in key order, across record types, until
// every vector is moved.
func TestBatchesOfTwoReachEveryVector(t *testing.T) {
	e := integration.Setup(t)
	for _, entityType := range []string{"lead", "contact", "company", "deal", "contact"} {
		storedAtFullPrecision(t, e, entityType)
	}
	ctx := systemCtx(e)
	in := datatask.BatchIn{Limit: 2}
	for batches := 1; ; batches++ {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) (err error) {
			out, err = DataTask().Steps[0].Batch(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatalf("batch %d: %v", batches, err)
		}
		if out.Changed > 2 {
			t.Fatalf("batch %d moved %d vectors, over its limit of 2", batches, out.Changed)
		}
		if out.Done {
			break
		}
		if batches > 5 {
			t.Fatal("the walk did not finish in the batches five vectors need")
		}
		in.Cursor = out.Cursor
	}
	if n := atFullPrecision(t, e); n != 0 {
		t.Errorf("%d vector(s) were never reached", n)
	}
}

// A batch that fails after its move leaves every vector where it was, and a
// cursor that is no key is refused.
func TestTheTaskRollsBackWithItsBatchAndRefusesAMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	storedAtFullPrecision(t, e, "contact")
	ctx := systemCtx(e)
	task := DataTask()
	move := task.Steps[0].Batch
	interrupted := false
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := move(ctx, tx, in)
		if err != nil {
			return out, err
		}
		interrupted = out.Changed == 1
		return out, errors.New("interrupt after the move")
	}
	e.RunDataTask(ctx, t, task)
	if !interrupted {
		t.Fatal("the batch failed before its move, so no rollback was tested")
	}
	if n := atFullPrecision(t, e); n != 1 {
		t.Errorf("the rolled-back batch left %d vector(s) at full precision, want 1", n)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := move(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 2})
		return err
	}); err == nil {
		t.Fatal("accepted a malformed cursor")
	}
}
