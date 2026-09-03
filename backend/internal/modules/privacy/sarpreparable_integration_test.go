// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// Every Art. 15 section's statement is valid SQL against the real schema.
//
// A section is not optional. The gather runs them in order and the first
// failure aborts the whole assembly, so ONE query naming a column that does not
// exist means no subject can get an access package at all — not a thinner one,
// none. That shipped: a section selected `captured_at` from
// consent_qualifying_event, which records occurred_at and created_at and has
// never had a captured_at, and every export failed with SQLSTATE 42703.
//
// The behavioural suites could not catch it in the direction that mattered.
// They assert on what a package CONTAINS, so they fail late, loudly, and
// identically for every defect — which is what a reader sees when the one
// broken section is somebody else's chapter. This asks the narrower question
// straight at the database: does the statement parse and resolve.
//
// PREPARE, not execute. Preparing resolves every table and column name and
// checks the argument arity, which is the whole class of defect here, and does
// it without seeding a subject or reading a row — so this stays a schema check
// rather than a second copy of the export's behaviour.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEverySARSectionStatementResolvesAgainstTheSchema(t *testing.T) {
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	// The real gather list, built the way the export builds it. Asking
	// sarSections rather than listing queries here is the point: a section
	// added to any chapter is covered the day it is added, and one that stops
	// being reachable stops being checked, which is correct in both directions.
	var pkg SARPackage
	sections := sarSections(&pkg, ids.New[ids.PersonKind](), []string{"subject@sar.test"}, []ids.UUID{ids.NewV7()})

	// A floor, because a gather list that came back empty would report the same
	// clean pass as one that resolved every statement. The number is the shape
	// of the list rather than its exact length — this is not a census of which
	// sections exist, which sarcoverage holds.
	const atLeast = 20
	if len(sections) < atLeast {
		t.Fatalf("the gather list has %d section(s), fewer than the %d this check assumes — "+
			"it was about to pass having resolved almost nothing", len(sections), atLeast)
	}

	for i, section := range sections {
		if _, err := owner.Prepare(ctx, "sar_section_probe_"+strings.TrimSpace(itoa(i)), section.query); err != nil {
			t.Errorf("section %d does not resolve against the schema: %v\n\n%s",
				i, err, strings.TrimSpace(section.query))
		}
	}
}

// itoa avoids pulling strconv in for one call site in a file that otherwise
// touches no formatting.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
