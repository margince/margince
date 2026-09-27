// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The per-number phone evidence migration, replayed over rows written under
// the one-row-per-field schema it replaces. Both texts are the shipped FILES.

import (
	"context"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const everyNumberMigration = "1790478765_every_phone_number_is_its_own_evidence"

var everyNumberContact = ids.MustParse("01920000-0000-7000-8000-0000000000f1")

// A phone row written before the migration keys on the number the writer now
// derives, so the next statement of that number lands on it rather than
// beside it; a single-answer field keys on nothing.
func TestTheBackfillKeysAStoredNumberTheWayTheWriterWill(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	replayDown(t, conn, everyNumberMigration)
	seedEveryNumberContact(t, conn)
	if _, err := conn.Exec(ctx, `
		INSERT INTO contact_profile_field (contact_id, field, value, evidence_snippet, source_ref, source, captured_by)
		VALUES ($1, 'phone', '0049 (30) 555-0101', 'Tel 0049 (30) 555-0101', 'activity:x', 'capture_enrich', 'agent:enrich'),
		       ($1, 'title', 'CTO', 'CTO', 'activity:x', 'capture_enrich', 'agent:enrich')`,
		everyNumberContact); err != nil {
		t.Fatalf("seeding the one-row-per-field evidence: %v", err)
	}
	replayMigration(t, conn, everyNumberMigration+".up.sql")

	if got := valueKeys(t, conn); !maps.Equal(got, map[string]string{"phone": "+49305550101", "title": ""}) {
		t.Fatalf("value keys = %v, want the phone keyed on its E.164 form and the title on nothing", got)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO contact_profile_field (contact_id, field, value_key, value, evidence_snippet, source_ref, source, captured_by)
		VALUES ($1, 'phone', '+6595550102', '+6595550102', '+65 9555 0102', 'activity:x', 'capture_enrich', 'agent:enrich')`,
		everyNumberContact); err != nil {
		t.Fatalf("a second number beside the first = %v, want it admitted: a phone is a list", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO contact_profile_field (contact_id, field, value_key, value, evidence_snippet, source_ref, source, captured_by)
		VALUES ($1, 'title', 'x', 'CEO', 'CEO', 'activity:x', 'capture_enrich', 'agent:enrich')`,
		everyNumberContact); err == nil {
		t.Fatal("a keyed title was admitted, want the cardinality check to hold every other field to one row")
	}
}

// Going back to one row per field keeps each contact's most recently stated
// number, so the restored unique constraint can be built at all.
func TestTheRollbackKeepsTheNewestNumber(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	seedEveryNumberContact(t, conn)
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO contact_profile_field
		  (contact_id, field, value_key, value, evidence_snippet, source_ref, source, captured_by, observed_at)
		VALUES ($1, 'phone', '+49305550101', '+49305550101', 'a', 'activity:x', 'capture_enrich', 'agent:enrich', now() - interval '2 days'),
		       ($1, 'phone', '+6595550102', '+6595550102', 'b', 'activity:x', 'capture_enrich', 'agent:enrich', now() - interval '1 day')`,
		everyNumberContact); err != nil {
		t.Fatalf("seeding two numbers: %v", err)
	}
	replayDown(t, conn, everyNumberMigration)

	rows, err := conn.Query(context.Background(),
		`SELECT value FROM contact_profile_field WHERE contact_id = $1 AND field = 'phone'`, everyNumberContact)
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(numbers) != 1 || numbers[0] != "+6595550102" {
		t.Fatalf("numbers after the rollback = %v, want only the newer one", numbers)
	}
}

func replayDown(t *testing.T, conn *pgx.Conn, name string) {
	t.Helper()
	replayMigration(t, conn, name+".down.sql")
}

func seedEveryNumberContact(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Nia Numbers', 'manual', 'human:test')`, everyNumberContact); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
}

func valueKeys(t *testing.T, conn *pgx.Conn) map[string]string {
	t.Helper()
	rows, err := conn.Query(context.Background(),
		`SELECT field, value_key FROM contact_profile_field WHERE contact_id = $1`, everyNumberContact)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var field, key string
		if err := rows.Scan(&field, &key); err != nil {
			t.Fatal(err)
		}
		out[field] = key
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
