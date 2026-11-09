// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAdmittedTransactionKeepsItsSlotAfterTheLeaseBackendDies(t *testing.T) {
	ctx, pool := laneEnv(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	slot := "test:slot:" + ids.NewV7().String()
	owner, held, err := database.TrySessionLease(ctx, pool, slot+":first", log)
	if err != nil || !held {
		t.Fatalf("first lease: held=%v err=%v", held, err)
	}
	defer owner.Close(ctx)
	if held, err = owner.TryLock(ctx, slot, false); err != nil || !held {
		t.Fatalf("exclusive slot: %v %v", held, err)
	}
	if held, err = owner.TryLock(ctx, slot, true); err != nil || !held {
		t.Fatalf("shared slot: %v %v", held, err)
	}
	if err = owner.Unlock(ctx, slot, false); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	args := []any{slot}
	if _, err = tx.Exec(ctx, fmt.Sprintf("SELECT pg_advisory_xact_lock_shared(hashtextextended($%d,0))", len(args)), args...); err != nil {
		t.Fatal(err)
	}
	args = []any{owner.PID()}
	var terminated bool
	if err = pool.QueryRow(ctx, fmt.Sprintf("SELECT pg_terminate_backend($%d, 5000)", len(args)), args...).Scan(&terminated); err != nil || !terminated {
		t.Fatalf("terminate: %v %v", terminated, err)
	}
	owner.Close(ctx)
	next, held, err := database.TrySessionLease(ctx, pool, slot+":second", log)
	if err != nil || !held {
		t.Fatalf("replacement lease: %v %v", held, err)
	}
	defer next.Close(ctx)
	if held, err = next.TryLock(ctx, slot, false); err != nil || held {
		t.Fatalf("overlapped admitted transaction: %v %v", held, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if held, err = next.TryLock(ctx, slot, false); err != nil || !held {
		t.Fatalf("slot not returned: %v %v", held, err)
	}
}

func TestTransactionOwnershipIsCheckedOnSnapshotJoinsAndDetachedWrites(t *testing.T) {
	ctx, pool := laneEnv(t)
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		t.Fatal("workspace not bound")
	}
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
	lost := errors.New("execution no longer owns its slot")
	checks := 0
	permitted := true
	ctx = database.WithTransactionGuard(ctx, func(context.Context, pgx.Tx) error {
		checks++
		if !permitted {
			return lost
		}
		return nil
	})
	called := 0
	write := func(tx pgx.Tx) error { called++; _, err := tx.Exec(ctx, `SELECT 1`); return err }
	if err := db.Tx(ctx, write); err != nil {
		t.Fatal(err)
	}
	if checks != 1 || called != 1 {
		t.Fatalf("guard=%d work=%d", checks, called)
	}
	err := database.WithWorkspaceSnapshot(ctx, pool, func(snapshot context.Context) error {
		before := checks
		if err := db.Tx(snapshot, write); err != nil {
			return err
		}
		if err := database.WithWorkspaceTx(snapshot, pool, write); err != nil {
			return err
		}
		if checks != before+2 {
			t.Fatalf("snapshot bypassed guard: %d to %d", before, checks)
		}
		permitted = false
		for _, run := range []func(context.Context, func(pgx.Tx) error) error{db.Tx, func(ctx context.Context, fn func(pgx.Tx) error) error { return database.WithWorkspaceTx(ctx, pool, fn) }} {
			beforeWork := called
			if err := run(context.WithoutCancel(snapshot), write); !errors.Is(err, lost) {
				t.Fatalf("stale snapshot: %v", err)
			}
			if called != beforeWork {
				t.Fatal("stale transaction ran domain work")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeWork := called
	if err := db.Tx(context.WithoutCancel(ctx), write); !errors.Is(err, lost) {
		t.Fatalf("detached stale execution: %v", err)
	}
	if called != beforeWork {
		t.Fatal("detached execution ran domain work")
	}
	beforeChecks := checks
	if err := db.Tx(database.WithoutTransactionGuard(ctx), write); err != nil {
		t.Fatal(err)
	}
	if checks != beforeChecks || called != beforeWork+1 {
		t.Fatalf("late credit scope: guard=%d work=%d", checks, called)
	}
}
