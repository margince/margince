// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

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
	ctx, meter := database.WithStatementMeter(ctx)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		var during, nested string
		err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			before := meter.Cost()
			if err := database.ReuseGenericPlans(ctx, func(context.Context) error { return nil }); err != nil {
				return err
			}
			if calls := meter.Cost().Minus(before).Exchanges; calls != 0 {
				t.Errorf("nested generic scope used %d pgx calls, want none", calls)
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

func TestCustomPlanExceptionRestoresTheWholeReadMode(t *testing.T) {
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			if err := database.UseCustomPlansTx(ctx, tx, func(ctx context.Context) error {
				if database.GenericPlansEnabled(ctx) {
					t.Error("the custom exception still reports generic plans")
				}
				if mode := planCacheMode(ctx, t, tx); mode != "force_custom_plan" {
					t.Errorf("exception mode = %q, want force_custom_plan", mode)
				}
				if err := database.ReuseGenericPlans(ctx, func(generic context.Context) error {
					if !database.GenericPlansEnabled(generic) || planCacheMode(generic, t, tx) != "force_generic_plan" {
						t.Error("nested generic scope did not override the custom exception")
					}
					return nil
				}); err != nil {
					return err
				}
				if mode := planCacheMode(ctx, t, tx); mode != "force_custom_plan" {
					t.Errorf("exception mode after nested generic scope = %q, want force_custom_plan", mode)
				}
				return nil
			}); err != nil {
				return err
			}
			if mode := planCacheMode(ctx, t, tx); mode != "force_generic_plan" {
				t.Errorf("mode after exception = %q, want force_generic_plan", mode)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if mode := planCacheMode(ctx, t, tx); mode != "auto" {
			t.Errorf("mode after read = %q, want auto", mode)
		}
	})
}

func TestDetachedReadDoesNotInheritTheOuterGenericPlanScope(t *testing.T) {
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			if !database.GenericPlansEnabled(ctx) {
				t.Fatal("outer snapshot did not enable generic plans")
			}
			return database.WithWorkspaceSnapshot(database.Detached(ctx), pool, func(detached context.Context) error {
				if database.GenericPlansEnabled(detached) {
					t.Error("detached snapshot inherited the outer transaction's plan scope")
				}
				return nil
			})
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

// The restore puts back what the snapshot had, not a default.
func TestReusedPlansRestoreTheModeTheSnapshotHad(t *testing.T) {
	ctx, pool := laneEnv(t)
	onSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		if _, err := tx.Exec(ctx, `SELECT set_config('plan_cache_mode', 'force_custom_plan', true)`); err != nil {
			t.Fatal(err)
		}
		if err := database.ReuseGenericPlans(ctx, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if mode := planCacheMode(ctx, t, tx); mode != "force_custom_plan" {
			t.Errorf("plan_cache_mode after the call = %q, want the snapshot's force_custom_plan", mode)
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

func TestExplicitPlanReuseRestoresTheCallerModeAfterARefusal(t *testing.T) {
	ctx, pool := laneEnv(t)
	refused := errors.New("refused")
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL plan_cache_mode=force_custom_plan`); err != nil {
			return err
		}
		var during string
		err := database.ReuseGenericPlansTx(ctx, tx, func() error {
			during = planCacheMode(ctx, t, tx)
			return refused
		})
		if !errors.Is(err, refused) || during != "force_generic_plan" {
			t.Fatalf("mode=%q error=%v, want generic planning and the caller's refusal", during, err)
		}
		if after := planCacheMode(ctx, t, tx); after != "force_custom_plan" {
			t.Fatalf("mode after refusal=%q, want the caller's custom mode", after)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The mode is the transaction's alone. After a snapshot that ended in a failed
// statement, every pooled connection plans as usual for the next request.
func TestReusedPlansNeverReachTheNextRequestOnTheConnection(t *testing.T) {
	ctx, pool := laneEnv(t)
	err := database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
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
	idle := pool.AcquireAllIdle(ctx)
	if len(idle) == 0 {
		t.Fatal("no idle connection to ask; the snapshot's connection was not returned to the pool")
	}
	for _, conn := range idle {
		var mode string
		if err := conn.QueryRow(ctx, `SELECT current_setting('plan_cache_mode')`).Scan(&mode); err != nil {
			t.Fatal(err)
		}
		conn.Release()
		if mode != "auto" {
			t.Errorf("plan_cache_mode on a pooled connection = %q, want auto", mode)
		}
	}
}
