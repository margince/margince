// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
)

// RecordLogoIntent declares a mark provisional before its bytes are put, taking the
// BASE the caller minted and applying the suffix PutLogo stores under.
//
// The suffix rather than the base, because that is the key the bytes land at — and
// the layout is this module's own, so the two writers that mint a base (the upload
// handler and the site-read resolve lane) do not have to know it. Both accept the
// orphan a failed persist leaves rather than delete an object the row may already
// name, which is what gives the ledger something to collect.
func (s *Store) RecordLogoIntent(ctx context.Context, base string) error {
	return storedobjects.NewLedger(s.db).Record(ctx, storedobjects.KindLogo, base+trimmedLogoSuffix)
}

// LogoKeyColumns is every column that may hold a mark's key, by table.
//
// ALL FOUR, because a reference check that misses one calls a live mark unreferenced
// and the sweep then deletes the bytes behind somebody's logo. A company carries a
// wide mark and an icon, and a site read carries the pair it resolved before any
// company adopted them.
//
// The query below is built from this list, so the columns are named once.
//
// Held by: TestTheMarkReferenceCheckReadsEveryMarkColumn (backend/gates/logokeycolumns_test.go)
// — it reads the committed schema and fails both ways: a mark column this list omits,
// and a column named here that the schema does not have.
var LogoKeyColumns = map[string][]string{
	companyEntity: {"logo_object_key", "logo_icon_object_key"},
	"site_read":   {"logo_object_key", "logo_icon_object_key"},
}

// markTableClauses spells each table's read as a LITERAL, so `FROM company` appears
// in this source and the company-read census can see it.
//
// Derived column lists with an interpolated table name would read as a company read
// to nobody: the census matches SQL text, and a table name assembled at runtime
// leaves none. A read this file genuinely makes must stay visible to the gate that
// polices them, even when the gate would then rule it harmless.
var markTableClauses = map[string]string{
	companyEntity: "NOT EXISTS (SELECT 1 FROM company t WHERE %s)",
	"site_read":   "NOT EXISTS (SELECT 1 FROM site_read t WHERE %s)",
}

// unreferencedMarkQuery builds the check from LogoKeyColumns.
//
// Only the COLUMNS are assembled, and every one is a compile-time literal in this
// file, never a value off a request.
func unreferencedMarkQuery() string {
	var clauses []string
	for _, table := range sortedTables(LogoKeyColumns) {
		var columns []string
		for _, column := range LogoKeyColumns[table] {
			columns = append(columns, fmt.Sprintf("t.%s = k", column))
		}
		clauses = append(clauses, fmt.Sprintf(markTableClauses[table], strings.Join(columns, " OR ")))
	}
	return "SELECT k FROM unnest($1::text[]) AS k WHERE " + strings.Join(clauses, "\n\t\t\t   AND ")
}

// UnreferencedLogoKeys answers which of these keys no mark column carries.
func (s *Store) UnreferencedLogoKeys(ctx context.Context, keys []string) ([]string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	var out []string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, unreferencedMarkQuery(), keys)
		if err != nil {
			return fmt.Errorf("check which mark keys are unreferenced: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return fmt.Errorf("read an unreferenced mark key: %w", err)
			}
			out = append(out, key)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// sortedTables keeps the statement stable across runs, so a plan cache and a reader
// both see one query rather than one per map iteration.
func sortedTables(by map[string][]string) []string {
	tables := make([]string, 0, len(by))
	for table := range by {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}
