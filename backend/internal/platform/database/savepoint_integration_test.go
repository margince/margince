// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

// A savepoint against a real server: what a failed write leaves on the shared
// transaction is a server fact, which no fake transaction can answer.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
)

const uniqueViolation = "23505"

func inSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool, fn func(context.Context, pgx.Tx)) error {
	t.Helper()
	return database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
		return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			fn(ctx, tx)
			return nil
		})
	})
}

func keyedTable(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE savepoint_keys (k int PRIMARY KEY) ON COMMIT DROP`); err != nil {
		t.Fatalf("creating the probe table: %v", err)
	}
}

func keyCount(ctx context.Context, t *testing.T, tx pgx.Tx) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM savepoint_keys`).Scan(&n); err != nil {
		t.Fatalf("the shared transaction cannot be read after the savepoint: %v", err)
	}
	return n
}

func TestAFailedWriteUnderASavepointLeavesTheSharedTransactionUsable(t *testing.T) {
	ctx, pool := laneEnv(t)
	err := inSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		keyedTable(ctx, t, tx)
		if _, err := tx.Exec(ctx, `INSERT INTO savepoint_keys VALUES (1)`); err != nil {
			t.Fatal(err)
		}
		failed := database.WithSavepoint(ctx, func(ctx context.Context) error {
			_, err := tx.Exec(ctx, `INSERT INTO savepoint_keys VALUES (2), (1)`)
			return err
		})
		if pgErr, ok := errors.AsType[*pgconn.PgError](failed); !ok || pgErr.Code != uniqueViolation {
			t.Fatalf("savepoint answered %v, want the unique violation of its own write", failed)
		}
		if got := keyCount(ctx, t, tx); got != 1 {
			t.Fatalf("rows after the failed write = %d, want the 1 written before the savepoint", got)
		}
	})
	if err != nil {
		t.Fatalf("the shared transaction failed to commit after a rolled-back savepoint: %v", err)
	}
}

func TestASuccessfulWriteUnderASavepointStaysInTheSharedTransaction(t *testing.T) {
	ctx, pool := laneEnv(t)
	err := inSnapshot(ctx, t, pool, func(ctx context.Context, tx pgx.Tx) {
		keyedTable(ctx, t, tx)
		for _, key := range []string{"1", "2"} {
			if err := database.WithSavepoint(ctx, func(ctx context.Context) error {
				_, err := tx.Exec(ctx, `INSERT INTO savepoint_keys VALUES (`+key+`)`)
				return err
			}); err != nil {
				t.Fatalf("savepoint for key %s: %v", key, err)
			}
		}
		if got := keyCount(ctx, t, tx); got != 2 {
			t.Fatalf("rows = %d, want both writes kept", got)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWithoutASharedTransactionTheSavepointJustRuns(t *testing.T) {
	ctx, _ := laneEnv(t)
	ran := false
	if err := database.WithSavepoint(ctx, func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("ran = %v, err = %v, want fn run once and its nil answer", ran, err)
	}
}

func TestASavepointThatCannotOpenOrCloseSaysSo(t *testing.T) {
	ctx, pool := laneEnv(t)
	cases := map[string]struct {
		run  func(ctx context.Context, tx pgx.Tx, cancel context.CancelFunc) error
		want string
	}{
		"opening": {
			run: func(ctx context.Context, _ pgx.Tx, cancel context.CancelFunc) error {
				cancel()
				return database.WithSavepoint(ctx, func(context.Context) error { return nil })
			},
			want: "opening a savepoint",
		},
		"releasing": {
			run: func(ctx context.Context, tx pgx.Tx, _ context.CancelFunc) error {
				return database.WithSavepoint(ctx, func(ctx context.Context) error {
					_, err := tx.Exec(ctx, `RELEASE SAVEPOINT with_savepoint`)
					return err
				})
			},
			want: "releasing the savepoint",
		},
		"rolling back": {
			run: func(ctx context.Context, _ pgx.Tx, cancel context.CancelFunc) error {
				return database.WithSavepoint(ctx, func(context.Context) error {
					cancel()
					return errors.New("the write failed")
				})
			},
			want: "rolling back to the savepoint",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cancelled, cancel := context.WithCancel(ctx)
			defer cancel()
			var got error
			snapshotErr := inSnapshot(cancelled, t, pool, func(ctx context.Context, tx pgx.Tx) {
				got = tc.run(ctx, tx, cancel)
			})
			t.Logf("the snapshot itself ended with: %v", snapshotErr)
			if got == nil || !strings.Contains(got.Error(), tc.want) {
				t.Fatalf("savepoint answered %v, want an error naming %q", got, tc.want)
			}
		})
	}
}
