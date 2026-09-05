// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Carrying the consent record the engine was never given.
//
// The rows under test are the ones the OLD writers produced — a
// consent_qualifying_event, a withdrawn person_consent — because that is the
// only shape a deployed database holds. Seeding the NEW tables and checking they
// are still there would pass against a migration that does nothing.
//
// It applies the actual migration file. A test that retyped the SQL would pass
// for a migration that no longer says it.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const engineBackfillMigration = "core/1788527759_the_engine_inherits_the_record_it_was_given.up.sql"

// asTheSchemaWasWhenTheBackfillShipped removes the columns added to
// communication_suppression AFTER this backfill, so replaying it here exercises
// the migration as it actually ran.
//
// These tests apply a shipped migration on top of a HEAD schema, which is a
// situation production never sees: in production the backfill ran first and the
// later columns were added to the rows it had already written. Without this the
// replay fails on a NOT NULL column that did not exist when the file was
// authored, and the failure says nothing about the backfill.
//
// Reversing rather than teaching the test each new column: a column added later
// is by definition one this migration cannot name, so the list below only ever
// grows in one direction and a missing entry fails loudly at the INSERT.
func asTheSchemaWasWhenTheBackfillShipped(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := conn.Exec(context.Background(),
		`ALTER TABLE communication_suppression DROP COLUMN IF EXISTS decided_by_level`); err != nil {
		t.Fatalf("restoring the schema the backfill was written against: %v", err)
	}
}

// backfillFixture is one person and the pre-engine rows recorded about them.
type backfillFixture struct {
	person      string
	marketing   string
	operational string
}

// seedPreEngineRecord writes what the product wrote before the engine existed.
func seedPreEngineRecord(t *testing.T, conn *pgx.Conn) backfillFixture {
	t.Helper()
	ctx := context.Background()
	var f backfillFixture
	if err := conn.QueryRow(ctx,
		`INSERT INTO person (full_name, source, captured_by)
		 VALUES ('Backfill Subject', 'manual', 'human:seed') RETURNING id`).Scan(&f.person); err != nil {
		t.Fatalf("seeding the person: %v", err)
	}
	if err := conn.QueryRow(ctx,
		`INSERT INTO consent_purpose (key, label, class, requires_double_opt_in)
		 VALUES ('backfill_news', 'Newsletter', 'marketing', false) RETURNING id`).Scan(&f.marketing); err != nil {
		t.Fatalf("seeding the marketing purpose: %v", err)
	}
	if err := conn.QueryRow(ctx,
		`INSERT INTO consent_purpose (key, label, class, requires_double_opt_in)
		 VALUES ('backfill_ops', 'Account notices', 'transactional', false) RETURNING id`).Scan(&f.operational); err != nil {
		t.Fatalf("seeding the operational purpose: %v", err)
	}
	return f
}

// TestTheEngineInheritsAQualifyingEvent is the carry that gives the engine its
// history: a ground a rep recorded years before communication_basis existed.
func TestTheEngineInheritsAQualifyingEvent(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	f := seedPreEngineRecord(t, conn)

	var anchor string
	if err := conn.QueryRow(ctx,
		`INSERT INTO activity (kind, direction, subject, occurred_at, source, captured_by)
		 VALUES ('email', 'inbound', 'A question', now() - interval '30 days', 'capture', 'human:seed')
		 RETURNING id`).Scan(&anchor); err != nil {
		t.Fatalf("seeding the source activity: %v", err)
	}
	occurred := time.Now().Add(-30 * 24 * time.Hour)
	if _, err := conn.Exec(ctx,
		`INSERT INTO consent_qualifying_event
		   (person_id, kind, source_entity_type, source_entity_id, occurred_at, source, captured_by)
		 VALUES ($1, 'inbound_message', 'activity', $2, $3, 'derived', 'human:seed')`,
		f.person, anchor, occurred); err != nil {
		t.Fatalf("seeding the qualifying event: %v", err)
	}

	asTheSchemaWasWhenTheBackfillShipped(t, conn)
	applyMigrationFile(t, conn, engineBackfillMigration)

	var kind, note string
	var source *string
	var validUntil time.Time
	if err := conn.QueryRow(ctx,
		`SELECT kind, note, source_activity_id::text, valid_until
		   FROM communication_basis WHERE person_id = $1`, f.person).
		Scan(&kind, &note, &source, &validUntil); err != nil {
		t.Fatalf("the engine inherited no basis for a person who has one on file: %v", err)
	}
	if kind != "subject_initiated_correspondence" {
		t.Errorf("carried kind = %q, want subject_initiated_correspondence — a qualifying event IS the subject having started something", kind)
	}
	if source == nil || *source != anchor {
		t.Errorf("carried source_activity_id = %v, want the seeded anchor %s — a basis nobody can trace to a record proves nothing", source, anchor)
	}
	// The engine's own reply window, so a carried row and a live one expire on
	// the same rule rather than the carry outliving what the product would grant.
	want := occurred.Add(365 * 24 * time.Hour)
	if validUntil.Sub(want).Abs() > time.Minute {
		t.Errorf("carried valid_until = %s, want %s (occurred_at + the reply window)", validUntil, want)
	}
}

// TestACarriedGroundWithNoRecordBehindItIsNotCarried holds the plan's own rule:
// nothing is invented. An event whose source activity is gone proves nothing a
// reader could check, so it stays absent rather than becoming a basis nobody
// can trace.
func TestACarriedGroundWithNoRecordBehindItIsNotCarried(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	f := seedPreEngineRecord(t, conn)

	// An activity id that never existed. The old table's FK-less
	// source_entity_id is exactly how a deployed database ends up holding one.
	if _, err := conn.Exec(ctx,
		`INSERT INTO consent_qualifying_event
		   (person_id, kind, source_entity_type, source_entity_id, occurred_at, source, captured_by)
		 VALUES ($1, 'inbound_message', 'activity', gen_random_uuid(), now() - interval '10 days', 'derived', 'human:seed')`,
		f.person); err != nil {
		t.Fatalf("seeding the orphaned qualifying event: %v", err)
	}

	asTheSchemaWasWhenTheBackfillShipped(t, conn)
	applyMigrationFile(t, conn, engineBackfillMigration)

	var n int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM communication_basis WHERE person_id = $1`, f.person).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("carried %d basis rows for an event whose source is gone, want 0 — a ground nobody can check is not a ground", n)
	}
}

// TestAMarketingWithdrawalBecomesAnObjection is the suppression half.
func TestAMarketingWithdrawalBecomesAnObjection(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	f := seedPreEngineRecord(t, conn)

	withdrawn(t, conn, f.person, f.marketing)

	asTheSchemaWasWhenTheBackfillShipped(t, conn)
	applyMigrationFile(t, conn, engineBackfillMigration)

	var kind, source string
	if err := conn.QueryRow(ctx,
		`SELECT kind, source FROM communication_suppression WHERE person_id = $1`, f.person).
		Scan(&kind, &source); err != nil {
		t.Fatalf("a marketing withdrawal carried no objection: %v", err)
	}
	if kind != "marketing_objection" {
		t.Errorf("carried kind = %q, want marketing_objection", kind)
	}
	if source != "carried_from_person_consent" {
		t.Errorf("carried source = %q — the down migration identifies this row by it", source)
	}
}

// TestANonMarketingWithdrawalIsNotAnObjection is the assertion this whole
// migration is shaped around, and the one a careless version fails.
//
// communication_suppression is NOT purpose-scoped: liveSuppression takes the
// strongest live row for a person and applies it to EVERY category, and
// marketing_objection is in commsauthz.Absolute — no rollout mode softens it.
// So carrying every withdrawn person_consent row across would take somebody who
// unsubscribed from one newsletter and block their invoices, their contract
// notices and their security mail, permanently.
//
// This is not hypothetical: a development database on this machine already held
// a withdrawal against a non-marketing purpose when the migration was written.
//
// Mutation: drop `AND cp.class = 'marketing'` from the second INSERT and this
// fails with a suppression this person should never have carried.
func TestANonMarketingWithdrawalIsNotAnObjection(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	f := seedPreEngineRecord(t, conn)

	withdrawn(t, conn, f.person, f.operational)

	asTheSchemaWasWhenTheBackfillShipped(t, conn)
	applyMigrationFile(t, conn, engineBackfillMigration)

	var n int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM communication_suppression WHERE person_id = $1`, f.person).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("carried %d suppression(s) from a NON-marketing withdrawal, want 0 — Art. 21 is an objection to direct marketing, and this row would block their invoices forever", n)
	}
}

// TestTheCarryIsIdempotent. A migration runner can re-run a file after a failed
// deploy, and a second run that doubled somebody's history would be a
// correctness bug nothing else catches.
func TestTheCarryIsIdempotent(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	f := seedPreEngineRecord(t, conn)

	var anchor string
	if err := conn.QueryRow(ctx,
		`INSERT INTO activity (kind, direction, subject, occurred_at, source, captured_by)
		 VALUES ('email', 'inbound', 'A question', now() - interval '5 days', 'capture', 'human:seed')
		 RETURNING id`).Scan(&anchor); err != nil {
		t.Fatalf("seeding the source activity: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO consent_qualifying_event
		   (person_id, kind, source_entity_type, source_entity_id, occurred_at, source, captured_by)
		 VALUES ($1, 'inbound_message', 'activity', $2, now() - interval '5 days', 'derived', 'human:seed')`,
		f.person, anchor); err != nil {
		t.Fatalf("seeding the qualifying event: %v", err)
	}
	withdrawn(t, conn, f.person, f.marketing)

	asTheSchemaWasWhenTheBackfillShipped(t, conn)
	applyMigrationFile(t, conn, engineBackfillMigration)
	asTheSchemaWasWhenTheBackfillShipped(t, conn)
	applyMigrationFile(t, conn, engineBackfillMigration)

	for _, c := range []struct {
		table string
		sql   string
	}{
		{"communication_basis", `SELECT count(*) FROM communication_basis WHERE person_id = $1`},
		{"communication_suppression", `SELECT count(*) FROM communication_suppression WHERE person_id = $1`},
	} {
		var n int
		if err := conn.QueryRow(ctx, c.sql, f.person).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("%s holds %d rows after two runs, want 1", c.table, n)
		}
	}
}

// withdrawn records a withdrawal the way the pre-engine product recorded one:
// the person_consent state plus the consent_event proof behind it.
func withdrawn(t *testing.T, conn *pgx.Conn, person, purpose string) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.Exec(ctx,
		`INSERT INTO person_consent (person_id, purpose_id, state, source, captured_at)
		 VALUES ($1, $2, 'withdrawn', 'preference_center', now() - interval '2 days')`, person, purpose); err != nil {
		t.Fatalf("seeding the withdrawal: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO consent_event
		   (person_id, purpose_id, new_state, source, captured_by, captured_at, policy_text, policy_version)
		 VALUES ($1, $2, 'withdrawn', 'preference_center', 'human:subject',
		         now() - interval '2 days', 'You asked us to stop emailing you.', 'v1')`,
		person, purpose); err != nil {
		t.Fatalf("seeding the withdrawal proof: %v", err)
	}
}
