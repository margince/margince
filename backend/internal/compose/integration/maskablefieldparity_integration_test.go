// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The catalog of what a mask may name has to reach a MIGRATED database, not
// just a Go map.
//
// deals/maskablefields_test.go pins migrations/testdata/maskable_fields.txt to
// dealMaskableFields, the code that does the withholding. That leaves one join
// unproven, and it is the one an installation actually runs on: the migration
// that writes those pairs into maskable_field. A migration that seeded the
// wrong pair, or none, passes every unit test in this tree — and the effect is
// silent in the direction that matters. A pair missing from the table cannot be
// configured at all (the FK refuses the field_mask row), so an administrator
// who sets a mask is told; a pair the table offers and no code withholds is a
// mask that is accepted, stored, read back unchanged, and withholds nothing.
//
// So: the table against the fixture, and then the refusal itself, in both
// directions. A constraint that refused EVERY mask would satisfy the refusal
// half on its own, which is why the admitted case is here beside it.

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// maskableFixture is the same file deals/maskablefields_test.go renders, read
// from the other side of the migration.
const maskableFixture = "../../../migrations/testdata/maskable_fields.txt"

// foreignKeyViolation is what Postgres answers when a mask names a pair the
// catalog does not offer.
const foreignKeyViolation = "23503"

func TestTheMigratedCatalogOffersExactlyWhatTheCodeWithholds(t *testing.T) {
	e := apptest.SetupApp(t)
	ctx := context.Background()

	rows, err := e.Pool.Query(ctx, `SELECT object, field FROM maskable_field ORDER BY object, field`)
	if err != nil {
		t.Fatalf("reading maskable_field: %v", err)
	}
	defer rows.Close()
	var seeded []string
	for rows.Next() {
		var object, field string
		if err := rows.Scan(&object, &field); err != nil {
			t.Fatalf("scanning maskable_field: %v", err)
		}
		seeded = append(seeded, object+" "+field)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading maskable_field: %v", err)
	}

	want := maskableFixtureLines(t)
	if strings.Join(seeded, "\n") != strings.Join(want, "\n") {
		t.Fatalf("the migrated catalog and the enforcing code disagree.\n"+
			"maskable_field holds:\n  %s\nthe fixture holds:\n  %s\n\n"+
			"A pair only the code knows about cannot be configured at all; a pair only the "+
			"table knows about is a mask an administrator sets and nothing applies.",
			strings.Join(seeded, "\n  "), strings.Join(want, "\n  "))
	}
	if len(seeded) == 0 {
		t.Fatal("maskable_field is empty — nothing can be masked on this installation, and " +
			"the comparison above would have agreed with an empty fixture just as happily")
	}
}

func TestAMaskNamingSomethingNothingWithholdsIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	ctx := context.Background()

	// The object is real and the field is not: the near miss, and the one a
	// constraint checking only the object would let through.
	_, err := e.Pool.Exec(ctx,
		`INSERT INTO field_mask (role_key, object, field, condition) VALUES ($1, $2, $3, $4)`,
		"rep", "deal", "close_date", "always")
	var refused *pgconn.PgError
	if !errors.As(err, &refused) || refused.Code != foreignKeyViolation {
		t.Fatalf("a mask on deal.close_date — a column nothing withholds — was stored: %v.\n"+
			"An accepted mask that withholds nothing is read back unchanged by the "+
			"administrator who set it, which is the belief this constraint exists to refuse.", err)
	}

	// A whole object nothing masks, which is where the tree actually is: the
	// helper in platform/auth is generic and only the deal calls it.
	_, err = e.Pool.Exec(ctx,
		`INSERT INTO field_mask (role_key, object, field, condition) VALUES ($1, $2, $3, $4)`,
		"rep", "contact", "phone", "always")
	if !errors.As(err, &refused) || refused.Code != foreignKeyViolation {
		t.Fatalf("a mask on contact.phone — an object no reader masks — was stored: %v", err)
	}

	// And the constraint still admits what the code enforces. Without this the
	// two refusals above are satisfied by a rule that refuses everything, which
	// would take the deal's own masks with it.
	if _, err := e.Pool.Exec(ctx,
		`INSERT INTO field_mask (role_key, object, field, condition) VALUES ($1, $2, $3, $4)`,
		"rep", "deal", "amount_minor", "outside_write_authority"); err != nil {
		t.Fatalf("the mask the deal store applies was refused: %v", err)
	}
}

func maskableFixtureLines(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(maskableFixture)
	if err != nil {
		t.Fatalf("reading the maskable-field fixture: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return lines
}
