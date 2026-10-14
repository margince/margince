// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A step-up asked for the read allowance can no longer be answered. The migration
// that removes the allowance lets the expiry sweep close those rows, and leaves
// every other step-up alone.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestPendingReadStepUpsAreDueForExpiryAndNothingElseMoves(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1791987509")); err != nil {
		t.Fatalf("up to the version before: %v", err)
	}
	future := clocktest.Now(t).Add(24 * time.Hour)
	for _, row := range []struct {
		name, kind, status, counter string
	}{
		{"read step-up", "volume_release", "pending", "reads"},
		{"write step-up", "volume_release", "pending", "writes"},
		{"answered read step-up", "volume_release", "rejected", "reads"},
		{"another kind naming reads", "create_record", "pending", "reads"},
	} {
		decidedAt := "NULL"
		if row.status == "rejected" {
			decidedAt = "now()"
		}
		if _, err := conn.Exec(ctx, `
			INSERT INTO approval (kind, status, proposed_by, proposed_change, diff_hash, expires_at, summary, decided_at)
			VALUES ($1, $2, 'agent:test', jsonb_build_object('counter', $3::text), 'h', $4, $5, `+decidedAt+`)`,
			row.kind, row.status, row.counter, future, row.name); err != nil {
			t.Fatalf("seeding %s: %v", row.name, err)
		}
	}

	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1791987600")); err != nil {
		t.Fatalf("up through the migration: %v", err)
	}

	for name, wantDue := range map[string]bool{
		"read step-up": true, "write step-up": false, "answered read step-up": false, "another kind naming reads": false,
	} {
		var due bool
		if err := conn.QueryRow(ctx, `SELECT expires_at <= now() FROM approval WHERE summary = $1`, name).Scan(&due); err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if due != wantDue {
			t.Errorf("%s: due for expiry = %v, want %v", name, due, wantDue)
		}
	}
}
