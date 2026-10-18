// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const unitKeyMigration = "core/1792363534_a_unit_is_a_key_the_reader_sees_translated"

type productUnit struct {
	unit    string
	version int
}

func productUnits(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]productUnit {
	t.Helper()
	rows, err := conn.Query(ctx, `SELECT name, unit, version FROM product`)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]productUnit{}
	for rows.Next() {
		var name string
		var got productUnit
		if err := rows.Scan(&name, &got.unit, &got.version); err != nil {
			t.Fatal(err)
		}
		out[name] = got
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// A unit typed as free text before the picklist reads as its key afterwards,
// in whichever interface language it was typed. One the list does not know
// keeps its text, and a row already holding its key is left untouched.
func TestEveryTypedUnitIsMappedToItsKeyOrKept(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	runFile(ctx, t, conn, unitKeyMigration+".down.sql")

	typed := map[string]string{
		"german day":    "Tag",
		"padded hour":   "  Stunde ",
		"english hours": "Hours",
		"vietnamese":    "Tháng",
		"flat":          "Pauschale",
		"already a key": "day",
		"unknown":       "Lizenz",
	}
	for name, unit := range typed {
		if _, err := conn.Exec(ctx, `
			INSERT INTO product (name, unit, unit_price_minor, currency, source, captured_by)
			VALUES ($1, $2, 100, 'EUR', 'manual', 'human:test')`, name, unit); err != nil {
			t.Fatalf("inserting %s: %v", name, err)
		}
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO product (name, unit_price_minor, currency, source, captured_by)
		VALUES ('old default', 100, 'EUR', 'manual', 'human:test')`); err != nil {
		t.Fatalf("inserting a product on the old default: %v", err)
	}
	before := productUnits(ctx, t, conn)

	runFile(ctx, t, conn, unitKeyMigration+".up.sql")

	got := productUnits(ctx, t, conn)
	want := map[string]string{
		"german day":    "day",
		"padded hour":   "hour",
		"english hours": "hour",
		"vietnamese":    "month",
		"flat":          "flat_fee",
		"already a key": "day",
		"unknown":       "Lizenz",
		"old default":   "piece",
	}
	for name, unit := range want {
		if got[name].unit != unit {
			t.Errorf("%s: unit %q, want %q", name, got[name].unit, unit)
		}
	}
	for _, name := range []string{"already a key", "unknown"} {
		if got[name].version != before[name].version {
			t.Errorf("%s: version moved from %d to %d, though its unit did not change", name, before[name].version, got[name].version)
		}
	}

	if _, err := conn.Exec(ctx, `
		INSERT INTO product (name, unit_price_minor, currency, source, captured_by)
		VALUES ('new default', 100, 'EUR', 'manual', 'human:test')`); err != nil {
		t.Fatalf("inserting a product on the new default: %v", err)
	}
	if unit := productUnits(ctx, t, conn)["new default"].unit; unit != "piece" {
		t.Errorf("a product naming no unit got %q, want piece", unit)
	}
}
