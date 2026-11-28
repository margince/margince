// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package storekit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type sweepPlace struct {
	After *ids.UUID `json:"after"`
	Round int       `json:"round"`
}

// stateUnderTest is a key no pass uses, so a test owns its row on a shared database.
func stateUnderTest(t *testing.T, pool *pgxpool.Pool) storekit.InstallationStateKey {
	t.Helper()
	key := storekit.InstallationStateKey("a_pass_under_test_" + ids.NewV7().String())
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM installation_state WHERE key = $1`, string(key)); err != nil {
			t.Errorf("removing the state under test: %v", err)
		}
	})
	return key
}

func inTx(t *testing.T, pool *pgxpool.Pool, fn func(ctx context.Context, tx pgx.Tx) error) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	if err := database.WithTx(ctx, pool, func(tx pgx.Tx) error { return fn(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
}

func TestAStateNobodyStoredReadsAsNotFound(t *testing.T) {
	pool := storePool(t)
	key := stateUnderTest(t, pool)
	inTx(t, pool, func(ctx context.Context, tx pgx.Tx) error {
		place, found, err := storekit.InstallationStateTx[sweepPlace](ctx, tx, key)
		if err != nil || found || place != (sweepPlace{}) {
			t.Errorf("an absent state read as (%+v, %t, %v), want the zero shape and not found", place, found, err)
		}
		return nil
	})
}

// A second store replaces the first, which an insert that kept the stored row would not.
func TestAStoredStateReadsBackInItsWritersShape(t *testing.T) {
	pool := storePool(t)
	key := stateUnderTest(t, pool)
	after := ids.NewV7()
	for _, stored := range []sweepPlace{{After: &after, Round: 2}, {Round: 3}} {
		inTx(t, pool, func(ctx context.Context, tx pgx.Tx) error {
			return storekit.StoreInstallationStateTx(ctx, tx, key, stored)
		})
		inTx(t, pool, func(ctx context.Context, tx pgx.Tx) error {
			got, found, err := storekit.InstallationStateTx[sweepPlace](ctx, tx, key)
			if err != nil || !found {
				t.Fatalf("reading back %+v: found=%t, %v", stored, found, err)
			}
			if got.Round != stored.Round || (got.After == nil) != (stored.After == nil) ||
				(got.After != nil && *got.After != *stored.After) {
				t.Errorf("stored %+v and read %+v", stored, got)
			}
			return nil
		})
	}
}

func TestLockingAStateStoresTheInitialShapeOnlyWhenNoneIsStored(t *testing.T) {
	pool := storePool(t)
	key := stateUnderTest(t, pool)
	for _, lock := range []struct{ initial, want int }{{initial: 1, want: 1}, {initial: 9, want: 1}} {
		inTx(t, pool, func(ctx context.Context, tx pgx.Tx) error {
			got, err := storekit.LockInstallationStateTx(ctx, tx, key, sweepPlace{Round: lock.initial})
			if err != nil || got.Round != lock.want {
				t.Errorf("locking with initial round %d answered (%+v, %v), want round %d", lock.initial, got, err, lock.want)
			}
			return nil
		})
	}
}

// Two runs of one pass take turns: while one holds the state, another cannot take it.
func TestALockedStateIsHeldUntilItsTransactionEnds(t *testing.T) {
	pool := storePool(t)
	key := stateUnderTest(t, pool)
	// Stored first: a row another transaction has not committed is one this probe cannot see.
	inTx(t, pool, func(ctx context.Context, tx pgx.Tx) error {
		return storekit.StoreInstallationStateTx(ctx, tx, key, sweepPlace{Round: 1})
	})
	inTx(t, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := storekit.LockInstallationStateTx(ctx, tx, key, sweepPlace{Round: 1}); err != nil {
			return err
		}
		_, err := pool.Exec(ctx, `SELECT 1 FROM installation_state WHERE key = $1 FOR UPDATE NOWAIT`, string(key))
		var refused *pgconn.PgError
		if !errors.As(err, &refused) || refused.Code != "55P03" {
			t.Errorf("a second run took the state while the first held it (%v), want lock_not_available", err)
		}
		return nil
	})
}

func TestAStateThatDoesNotFitItsShapeIsAnErrorNotAZeroShape(t *testing.T) {
	pool := storePool(t)
	key := stateUnderTest(t, pool)
	inTx(t, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO installation_state (key, value) VALUES ($1, '{"round":"two"}')`, string(key)); err != nil {
			return err
		}
		if _, found, err := storekit.InstallationStateTx[sweepPlace](ctx, tx, key); err == nil || found {
			t.Errorf("a state of another shape read as found=%t with no error", found)
		}
		return nil
	})
}
