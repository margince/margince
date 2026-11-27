// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// The seed skips triggers for its own inserts only. A caller's later writes
// in the same transaction, and every other connection, must still run them.
func TestSeedingAMailboxSkipsNoTriggerBeyondItsOwnInserts(t *testing.T) {
	t.Parallel()
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded mailbox: %v", err)
		}
	})
	other, err := pgx.Connect(ctx, testdb.For(t).OwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := other.Close(ctx); err != nil {
			t.Errorf("closing the second connection: %v", err)
		}
	})

	seedMailbox(ctx, t, tx, sparseMailbox)

	for name, q := range map[string]interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	}{"the seeding transaction": tx, "another connection": other} {
		var role string
		if err := q.QueryRow(ctx, `SHOW session_replication_role`).Scan(&role); err != nil {
			t.Fatalf("reading the role of %s: %v", name, err)
		}
		if role != "origin" {
			t.Errorf("%s runs with session_replication_role %q after the seed, want origin so triggers fire", name, role)
		}
	}
}
