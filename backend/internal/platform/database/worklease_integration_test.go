// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
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
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
