// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheColumnsRefuseAnOutsideIDWithNoSystemOrPastTheCap(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	seedLogoCompany(ctx, t, e, "Voltaq Systems GmbH", "voltaq.test")
	if _, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Anna Müller", Source: "manual"}); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	for _, table := range []string{"company", "contact"} {
		for name, tc := range map[string]struct {
			args       []any
			constraint string
		}{
			"an id with no system": {[]any{nil, "C-1"}, table + "_source_id_needs_a_source"},
			"an id past the cap":   {[]any{"acme-erp", strings.Repeat("x", provenance.SourceIDMax+1)}, table + "_source_id_length"},
			"an empty id":          {[]any{"acme-erp", ""}, table + "_source_id_length"},
		} {
			err := e.store.tx(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE `+table+` SET source_system = $1, source_id = $2 WHERE id = (SELECT id FROM `+table+` LIMIT 1)`, tc.args...)
				return err
			})
			if got, ok := storekit.CheckViolation(err); !ok || got != tc.constraint {
				t.Errorf("%s on %s: violated %q (%v), want %q", name, table, got, err, tc.constraint)
			}
		}
	}
}

// addsSourceID reads the tables the migration that added source_id gave it to.
// The migration owns that set, so a table it adds later joins the test.
var addsSourceID = regexp.MustCompile(`(?m)^ALTER TABLE (\w+)\s+ADD COLUMN source_id `)

// Every table that migration touched must keep both checks validated and the
// unique index. The deal and project tables are covered here, because a row to
// break their checks needs records this package does not seed.
func TestEveryTableGivenAnOutsideIDKeepsItsValidatedChecksAndIndex(t *testing.T) {
	files, err := filepath.Glob("../../../migrations/core/*_a_record_keeps_its_id_in_the_system_it_came_from.up.sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("finding the migration that added source_id: %v, %d files", err, len(files))
	}
	sql, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for _, m := range addsSourceID.FindAllStringSubmatch(string(sql), -1) {
		tables = append(tables, m[1])
	}
	if len(tables) < 4 {
		t.Fatalf("read %d tables from the migration, want at least the four record tables", len(tables))
	}

	e := setupDedupe(t)
	for _, table := range tables {
		var validated, indexed int
		if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
			return tx.QueryRow(e.as(), `
				SELECT (SELECT count(*) FROM pg_constraint
				        WHERE convalidated AND conname IN ($1 || '_source_id_needs_a_source', $1 || '_source_id_length')),
				       (SELECT count(*) FROM pg_indexes
				        WHERE tablename = $1 AND indexname = 'uq_' || $1 || '_source')`, table).Scan(&validated, &indexed)
		}); err != nil {
			t.Fatalf("reading the catalog for %s: %v", table, err)
		}
		if validated != 2 || indexed != 1 {
			t.Errorf("%s has %d of 2 validated source_id checks and %d of 1 unique index", table, validated, indexed)
		}
	}
}
