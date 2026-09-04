// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Retiring the seeded agent seat must not need the row to go away.
//
// Dozens of columns reference app_user(id) and many refuse a delete outright,
// `connected_by` on channel_connection among them — so a migration that DELETED
// the seat would fail the deploy on the first installation holding such a row.
// Every such column is actor-derived and the seat can never be an actor, so the
// product cannot write one; an operator or a repair script can, and a future
// runner seeded at this address certainly could. The migration therefore
// deactivates and archives.
//
// This runs the ACTUAL migration file rather than a statement retyped here. A
// test that restated the SQL would pass for a migration that no longer says it,
// which is the one failure mode a migration test has to avoid.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// retireAgentSeatMigration is the file both tests below apply.
const retireAgentSeatMigration = "core/1788508281_the_seeded_agent_seat_is_retired.up.sql"

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

	ownPassword := "$argon2id$fake"
	for _, tc := range []struct {
		name  string
		email string
		hash  *string
	}{
		{name: "carries a password of its own", email: "agent@real.gradion.local", hash: &ownPassword},
		{name: "answers at an address of its own", email: "runner@customer.example", hash: nil},
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

// TestRetiringTheAgentSeatRoundTrips holds the pair as an inverse.
//
// The up touches only a seat holding exactly (`active`, NULL), so the down
// restores exactly that pair and nothing has to record prior state. The second
// fixture is the boundary: a seat an operator DEACTIVATED but did not archive —
// the two are independent in this schema — is outside both predicates, so a
// rollback cannot hand it back as live.
//
// The case the pair genuinely cannot separate is a seat an operator deactivated
// AND archived by hand: it is indistinguishable from one this migration retired,
// so a rollback reactivates it. That is stated in the down migration, and it is
// the safe direction — the row comes back live but inert, since it holds no
// password, no role, and no passport can name it.
func TestRetiringTheAgentSeatRoundTrips(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	seat := seedRetirableSeat(t, conn, "agent@roundtrip.gradion.local", "active", nil)
	// The boundary: an operator stopped this one and did not archive it.
	byHand := seedRetirableSeat(t, conn, "agent@stopped.gradion.local", "deactivated", nil)
	// ACTIVE BUT ARCHIVED. The schema permits it, and the licence meter reads
	// `status` without reading `archived_at`, so this row is metered exactly like
	// any other and the up must retire it. A predicate that also demanded
	// `archived_at IS NULL` would skip it and leave the licence seat behind.
	archivedEarlier := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	metered := seedRetirableSeat(t, conn, "agent@metered.gradion.local", "active", &archivedEarlier)

	applyMigrationFile(t, conn, retireAgentSeatMigration)
	assertSeatState(t, conn, seat, "deactivated", true)
	// Skipped by the up: already out of the meter, so there is nothing to do to
	// it, and touching it would invent an archival the operator did not ask for.
	assertSeatState(t, conn, byHand, "deactivated", false)
	// Retired, and COALESCE kept the archival somebody else performed rather than
	// overwriting it with the migration's own clock.
	assertSeatState(t, conn, metered, "deactivated", true)
	var kept time.Time
	if err := conn.QueryRow(context.Background(),
		`SELECT archived_at FROM app_user WHERE id = $1`, metered).Scan(&kept); err != nil {
		t.Fatalf("reading the metered seat's archival: %v", err)
	}
	if !kept.Equal(archivedEarlier) {
		t.Errorf("archived_at moved to %s, want the existing %s — COALESCE must preserve it",
			kept, archivedEarlier)
	}

	applyMigrationFile(t, conn, strings.Replace(retireAgentSeatMigration, ".up.sql", ".down.sql", 1))
	// The one the up retired is back exactly as the up found it.
	assertSeatState(t, conn, seat, "active", false)
	// And the operator's own decision survives the rollback.
	assertSeatState(t, conn, byHand, "deactivated", false)
}

// seedRetirableSeat writes one seeded-shape agent seat in a given state.
func seedRetirableSeat(t *testing.T, conn *pgx.Conn, email, status string, archivedAt *time.Time) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(context.Background(),
		`INSERT INTO app_user (email, display_name, is_agent, seat_type, status, archived_at)
		 VALUES ($1, 'Margince Agent', true, 'full', $2, $3) RETURNING id`,
		email, status, archivedAt).Scan(&id); err != nil {
		t.Fatalf("seeding %s: %v", email, err)
	}
	return id
}

// assertSeatState reads one seat's status and whether it is archived.
func assertSeatState(t *testing.T, conn *pgx.Conn, id, wantStatus string, wantArchived bool) {
	t.Helper()
	var status string
	var archived bool
	if err := conn.QueryRow(context.Background(),
		`SELECT status, archived_at IS NOT NULL FROM app_user WHERE id = $1`, id).
		Scan(&status, &archived); err != nil {
		t.Fatalf("reading seat %s: %v", id, err)
	}
	if status != wantStatus || archived != wantArchived {
		t.Errorf("seat %s is (%s, archived=%v), want (%s, archived=%v)",
			id, status, archived, wantStatus, wantArchived)
	}
}

// applyMigrationFile executes one migration's SQL as the owner, inside a
// transaction, the way the migrate binary applies it — the `SET LOCAL
// lock_timeout` in these files is scoped to a transaction and is a no-op
// outside one.
//
// The path is namespace-relative and used as given, so `core/…` means core.
func applyMigrationFile(t *testing.T, conn *pgx.Conn, name string) {
	t.Helper()
	sql, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
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
