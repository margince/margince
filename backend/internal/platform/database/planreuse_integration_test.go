// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
)

func planCacheMode(ctx context.Context, t *testing.T, tx pgx.Tx) string {
	t.Helper()
	var mode string
	if err := tx.QueryRow(ctx, `SELECT current_setting('plan_cache_mode')`).Scan(&mode); err != nil {
		t.Fatalf("reading plan_cache_mode: %v", err)
	}
	return mode
}

func TestReusedPlansLastForTheCallAndTheSnapshotPlansAsBeforeAfterIt(t *testing.T) {
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		var during, nested string
		err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			if err := database.ReuseGenericPlans(ctx, func(context.Context) error { return nil }); err != nil {
				return err
			}
			nested = planCacheMode(ctx, t, tx)
			during = planCacheMode(ctx, t, tx)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if during != "force_generic_plan" || nested != "force_generic_plan" {
			t.Errorf("plan_cache_mode inside = %q, after a nested call = %q; want force_generic_plan for both", during, nested)
		}
		if mode := planCacheMode(ctx, t, tx); mode != "auto" {
			t.Errorf("plan_cache_mode after the call = %q, want auto", mode)
		}
	})
}

// A call that fails without aborting the transaction leaves the snapshot usable,
// so the statements after it must plan as they did before it.
func TestAFailedCallStillRestoresHowTheSnapshotPlans(t *testing.T) {
	ctx, pool := laneEnv(t)
	refused := errors.New("refused")
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		if err := database.ReuseGenericPlans(ctx, func(context.Context) error { return refused }); !errors.Is(err, refused) {
			t.Fatalf("ReuseGenericPlans = %v, want the call's own error", err)
		}
		if mode := planCacheMode(ctx, t, tx); mode != "auto" {
			t.Errorf("plan_cache_mode after a failed call = %q, want auto", mode)
		}
	})
}

// The mode is the transaction's alone: the next request on the same pooled
// connection plans as usual, even when the snapshot ended in a failed statement.
func TestReusedPlansNeverReachTheNextRequestOnTheConnection(t *testing.T) {
	ctx, _ := laneEnv(t)
	config, err := pgxpool.ParseConfig(os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	err = database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
		return database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `SELECT 1/0`)
				return err
			})
		})
	})
	if err == nil {
		t.Fatal("the failing statement did not fail the snapshot")
	}
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if mode := planCacheMode(ctx, t, tx); mode != "auto" {
			t.Errorf("plan_cache_mode on the next request = %q, want auto", mode)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
