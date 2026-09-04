// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Retiring the seeded agent seat must not need the row to go away.
//
// app_user(id) is referenced 92 times across the head schema, 16 of them ON
// DELETE RESTRICT, so a migration that DELETED the seat would fail the deploy on
// the first installation holding one of those rows — and `connected_by` on
// channel_connection is exactly that, because a connector configured against the
// seat names it directly. The migration therefore deactivates and archives.
//
// This runs the ACTUAL migration file rather than a statement retyped here. A
// test that restated the SQL would pass for a migration that no longer says it,
// which is the one failure mode a migration test has to avoid.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// retireAgentSeatMigration is the file under test, named once.
const retireAgentSeatMigration = "core/1788502500_the_seeded_agent_seat_is_retired.up.sql"

// TestRetiringTheAgentSeatSurvivesAReferencingRow is the case a DELETE fails on.
func TestRetiringTheAgentSeatSurvivesAReferencingRow(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	// The seat as bootstrap used to write it: an is_agent full seat with no
	// password, at the reserved address the migration's predicate names.
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name, is_agent, seat_type, status)
		 VALUES ('agent@retire-test.gradion.local', 'Margince Agent', true, 'full', 'active')
		 RETURNING id`).Scan(&seat); err != nil {
		t.Fatalf("seeding the agent seat: %v", err)
	}
	// The ON DELETE RESTRICT reference. This row is what makes the choice of
	// UPDATE over DELETE load-bearing rather than stylistic.
	if _, err := conn.Exec(ctx,
		`INSERT INTO channel_connection
		   (provider, channel_id, channel_label, credential_ref, status, connected_by)
		 VALUES ('telegram', 'chan-1', 'A connected channel', 'vault://k', 'connected', $1)`,
		seat); err != nil {
		t.Fatalf("seeding a channel_connection that names the seat: %v", err)
	}

	applyMigrationFile(t, conn, retireAgentSeatMigration)

	var status string
	var archived bool
	if err := conn.QueryRow(ctx,
		`SELECT status, archived_at IS NOT NULL FROM app_user WHERE id = $1`, seat).
		Scan(&status, &archived); err != nil {
		t.Fatalf("reading the seat back: %v", err)
	}
	// DEACTIVATED is what frees the licence seat: identity's meter filters on
	// status and never reads archived_at, so archiving alone would keep metering
	// it. ARCHIVED is what removes it from the roster and the live-member
	// predicates. Both are asserted because either alone leaves half the row's
	// cost in place.
	if status != "deactivated" {
		t.Errorf("seat status = %q, want deactivated — the licence meter filters on status, so any "+
			"other value keeps billing for it", status)
	}
	if !archived {
		t.Error("the seat is not archived, so it still appears in the roster and in every live-member read")
	}
	// And the referencing row is untouched: retiring an identity must not
	// destroy the provenance of what was configured under it.
	var connections int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM channel_connection WHERE connected_by = $1`, seat).Scan(&connections); err != nil {
		t.Fatalf("counting the referencing rows: %v", err)
	}
	if connections != 1 {
		t.Errorf("channel_connection rows naming the seat = %d, want 1 — the reference is provenance "+
			"and the retirement is not a deletion", connections)
	}
}

// TestRetiringTheAgentSeatLeavesARealAgentIdentityAlone holds the predicate's
// bound.
//
// An installation that has minted its own agent identity — one with a password,
// or at an address of its own — keeps it. The migration retires the row
// BOOTSTRAP wrote, identified by the NULL password and the reserved domain
// together, and a predicate that caught more than that would silently
// deactivate a runner somebody is using.
func TestRetiringTheAgentSeatLeavesARealAgentIdentityAlone(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		email string
		hash  *string
	}{
		{name: "carries a password", email: "agent@real.gradion.local", hash: strptr("$argon2id$fake")},
		{name: "address of its own", email: "runner@customer.example", hash: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var id string
			if err := conn.QueryRow(ctx,
				`INSERT INTO app_user (email, display_name, is_agent, seat_type, status, password_hash)
				 VALUES ($1, 'A Real Runner', true, 'full', 'active', $2) RETURNING id`,
				tc.email, tc.hash).Scan(&id); err != nil {
				t.Fatalf("seeding the agent identity: %v", err)
			}

			applyMigrationFile(t, conn, retireAgentSeatMigration)

			var status string
			if err := conn.QueryRow(ctx,
				`SELECT status FROM app_user WHERE id = $1`, id).Scan(&status); err != nil {
				t.Fatalf("reading the identity back: %v", err)
			}
			if status != "active" {
				t.Errorf("an agent identity that %s was retired (status %q); the predicate names the "+
					"row bootstrap wrote, and widening it deactivates a runner in use", tc.name, status)
			}
		})
	}
}

func strptr(s string) *string { return &s }

// applyMigrationFile executes one migration's SQL as the owner, inside a
// transaction, the way the migrate binary applies it — the `SET LOCAL
// lock_timeout` in these files is scoped to a transaction and is a no-op
// outside one.
func applyMigrationFile(t *testing.T, conn *pgx.Conn, name string) {
	t.Helper()
	sql, err := os.ReadFile(filepath.Join("core", filepath.Base(name)))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	if strings.TrimSpace(string(sql)) == "" {
		t.Fatalf("%s is empty — this test would prove nothing", name)
	}
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	//craft:ignore swallowed-errors error-path safety net only — the Commit below is asserted, after which this rollback is a designed no-op
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("applying %s: %v", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing %s: %v", name, err)
	}
}
