// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A writer of a capture fact cannot commit while a capture transaction that
// checked the version is still using the run's copy. It waits, and the next
// transaction sees what it wrote.
func TestAFactWriterWaitsForTheCaptureThatCheckedTheVersion(t *testing.T) {
	ctx, pool := laneEnv(t)
	run := storekit.WithCaptureFactsRun(ctx)
	domain := "barrier-" + ids.NewV7().String() + ".example"
	t.Cleanup(func() {
		if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM capture_freemail_domain WHERE domain = $1`, domain)
			return err
		}); err != nil {
			t.Error(err)
		}
	})
	listed := func(tx pgx.Tx) (int, error) {
		return storekit.RememberCaptureFact(run, tx, storekit.CaptureFactsMemo+"barrier", func() (int, error) {
			var n int
			err := tx.QueryRow(ctx, `SELECT count(*) FROM capture_freemail_domain WHERE domain = $1`, domain).Scan(&n)
			return n, err
		})
	}
	capture := func(during func(tx pgx.Tx)) int {
		var seen int
		if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			if err := storekit.HoldCaptureFactsTx(run, tx); err != nil {
				return err
			}
			if during != nil {
				during(tx)
			}
			var err error
			seen, err = listed(tx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return seen
	}
	if seen := capture(nil); seen != 0 {
		t.Fatalf("the list holds %d rows for a fresh domain", seen)
	}

	written := make(chan error, 1)
	waiting := capture(func(reader pgx.Tx) {
		writer := make(chan uint32, 1)
		go func() {
			written <- database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
				var pid uint32
				if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
					return err
				}
				writer <- pid
				if _, err := tx.Exec(ctx, `INSERT INTO capture_freemail_domain (domain, kind) VALUES ($1, 'extra')`, domain); err != nil {
					return err
				}
				return storekit.CaptureFactsChanged(ctx, tx)
			})
		}()
		pid := <-writer
		for blocked := false; !blocked; {
			if err := reader.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks
				 WHERE pid = $1 AND locktype = 'advisory' AND NOT granted)`, pid).Scan(&blocked); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-written:
				t.Fatalf("the writer committed (%v) while a capture held the facts lock", err)
			default:
			}
		}
	})
	if waiting != 0 {
		t.Fatalf("the capture that checked the version saw %d rows a waiting writer had not committed", waiting)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if seen := capture(nil); seen != 1 {
		t.Fatalf("the next capture saw %d rows after the writer committed, want 1: it used the old copy", seen)
	}
}
