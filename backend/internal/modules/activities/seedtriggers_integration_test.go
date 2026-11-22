// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// A seeded mailbox that left a trigger off would let a later write skip the
// settlement locks or the strength dates. The test would then prove less than
// it reads.
func TestSeedingAMailboxLeavesEveryTriggerItSilencedEnabled(t *testing.T) {
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
	seedMailbox(ctx, t, tx, sparseMailbox)

	for _, s := range seedSilencedTriggers {
		var enabled string
		err := tx.QueryRow(ctx, `SELECT tgenabled::text FROM pg_trigger WHERE tgrelid = $1::regclass AND tgname = $2`,
			pgx.Identifier{s.table}.Sanitize(), s.trigger).Scan(&enabled)
		if err != nil {
			t.Fatalf("reading trigger %s on %s: %v", s.trigger, s.table, err)
		}
		if enabled != "O" {
			t.Errorf("trigger %s on %s has tgenabled %q after the seed, want O (enabled)", s.trigger, s.table, enabled)
		}
	}
}
