// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWorkLeaseProofNamesOnlyItsLiveOwner(t *testing.T) {
	ctx, pool := laneEnv(t)
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		t.Fatal("workspace missing")
	}
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
	id := ids.NewV7()
	var saved context.Context
	held, err := db.WithWorkLease(ctx, "test", id, slog.Default(), func(owned context.Context) error {
		saved = owned
		for name, tc := range map[string]struct {
			ctx  context.Context
			kind string
			id   ids.UUID
			want bool
		}{
			"owner":        {owned, "test", id, true},
			"unowned":      {ctx, "test", id, false},
			"other kind":   {owned, "other", id, false},
			"other record": {owned, "test", ids.NewV7(), false},
		} {
			got, err := db.WorkLeaseOwned(tc.ctx, tc.kind, tc.id)
			if err != nil || got != tc.want {
				t.Errorf("%s: owned=%v err=%v", name, got, err)
			}
		}
		return db.Tx(owned, func(tx pgx.Tx) error {
			free, err := db.WorkLeaseAvailable(owned, tx, "test", id)
			if err != nil || free {
				t.Fatalf("live owner available=%v err=%v", free, err)
			}
			return nil
		})
	})
	if err != nil || !held {
		t.Fatalf("lease: held=%v err=%v", held, err)
	}
	if owned, err := db.WorkLeaseOwned(saved, "test", id); err != nil || owned {
		t.Fatalf("released owner: owned=%v err=%v", owned, err)
	}
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		free, err := db.WorkLeaseAvailable(ctx, tx, "test", id)
		if err != nil || !free {
			t.Fatalf("released owner available=%v err=%v", free, err)
		}
		lease, held, err := db.TryWorkLease(ctx, "test", id, slog.Default())
		if err != nil || !held {
			t.Fatalf("availability check blocked a worker: %v %v", held, err)
		}
		lease.Close(ctx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkLeaseLossCannotOvertakeAClaimOrItsAdmittedWrite(t *testing.T) {
	ctx, pool := laneEnv(t)
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		t.Fatal("workspace missing")
	}
	db := database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
	id := ids.NewV7()
	held, err := db.WithWorkLease(ctx, "test", id, slog.Default(), func(old context.Context) error {
		if owned, err := db.WorkLeaseOwned(old, "test", id); err != nil || !owned {
			t.Fatalf("initial ownership: %v %v", owned, err)
		}
		started, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() { done <- db.Tx(old, func(pgx.Tx) error { close(started); <-release; return nil }) }()
		<-started
		var killed int
		err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE pg_terminate_backend(pid, 5000))
   FROM pg_locks WHERE locktype = 'advisory' AND granted AND mode = 'ExclusiveLock'
   AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`, pgx.StrictNamedArgs{}).Scan(&killed)
		if err != nil || killed != 1 {
			close(release)
			<-done
			t.Fatalf("ending owner: %d %v", killed, err)
		}
		lease, acquired, acquireErr := db.TryWorkLease(ctx, "test", id, slog.Default())
		if lease != nil {
			lease.Close(ctx)
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if acquireErr != nil || acquired {
			t.Fatalf("replacement overtook an admitted write: %v %v", acquired, acquireErr)
		}
		called := false
		if err := db.Tx(context.WithoutCancel(old), func(pgx.Tx) error { called = true; return nil }); !errors.Is(err, apperrors.ErrConflict) || called {
			t.Fatalf("stale claim entered domain work: called=%v err=%v", called, err)
		}
		nextHeld, err := db.WithWorkLease(ctx, "test", id, slog.Default(), func(next context.Context) error {
			called = false
			if err := db.Tx(old, func(pgx.Tx) error { called = true; return nil }); !errors.Is(err, apperrors.ErrConflict) || called {
				t.Fatalf("stale execution replaced the new owner: called=%v err=%v", called, err)
			}
			return db.Tx(next, func(pgx.Tx) error { return nil })
		})
		if err != nil || !nextHeld {
			t.Fatalf("replacement: %v %v", nextHeld, err)
		}
		return nil
	})
	if err != nil || !held {
		t.Fatalf("first execution: %v %v", held, err)
	}
}
