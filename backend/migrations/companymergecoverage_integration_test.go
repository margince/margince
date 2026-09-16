// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Every foreign key pointing at company is a row the company merge has to
// decide about. This census derives them from the DATABASE and asks the merge
// path's own source what it does with each one, so a new table carrying a
// company_id is enrolled the moment its migration creates it — not when
// somebody remembers to add it to a list.
//
// The defect it exists to catch is silent in both directions. A column the
// merge forgets leaves its rows pointing at a record no read returns: nothing
// errors, and to a user the data has simply vanished from the survivor. A
// census that under-counts reports PASS over a tree it never read, which looks
// exactly the same as a clean one — hence theCompanyFKFloor below.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// theCompanyFKFloor is the count below which the census has broken rather than
// the schema having shrunk. 39 columns point at company today; a derivation
// that suddenly sees fewer than 35 is reading a smaller tree than it thinks,
// and would otherwise pass in silence.
const theCompanyFKFloor = 35

// mergePathFiles are the files that, between them, ARE the company merge's
// relink. The census reads their SQL rather than a list of tables, so a
// statement deleted from the merge fails here even though the waiver map and
// the schema are both untouched.
var mergePathFiles = []string{
	"internal/modules/contacts/mergerelink_company.go",
	"internal/modules/contacts/merge_company.go",
	"internal/modules/contacts/merge.go",
	"internal/modules/contacts/mergecompanyedges.go",
	"internal/modules/contacts/company_relationship_types.go",
}

// companyFKsTheMergeLeaves are the company-pointing columns the merge
// deliberately does not move, each with the reason moving it would be wrong.
//
// EMPTY, and that is the finding rather than an omission: every foreign key
// pointing at company today is one the merge actually writes. The map stays
// because the next column to arrive may genuinely belong here — a ledger row
// that must not move, say — and an entry with a stated cost is how that gets
// declared. gatekit reports an entry that stops matching, so a waiver added
// here cannot quietly outlive the column it was written for.
var companyFKsTheMergeLeaves = gatekit.Waive(map[string]string{})

// sqlWriteTarget matches the table a statement writes.
var sqlWriteTarget = regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+([a-z_][a-z0-9_]*)`)

// TestEveryCompanyForeignKeyJoinsTheMerge is the coverage census.
//
// It fails when a table carrying a company_id is neither written by the merge
// path nor declared above — which is the shape every defect in #5739 had: the
// enrichment sidecars, the money rows and the per-reader rows were all real
// columns in the schema that no statement in the merge had ever named.
func TestEveryCompanyForeignKeyJoinsTheMerge(t *testing.T) {
	defer companyFKsTheMergeLeaves.AssertAllMatched(t)

	written := tablesTheMergeWrites(t)
	// A source scan that found nothing would waive the whole census by
	// accident: every column would read as unmoved, the waiver map would not
	// cover them, and the failures would look like a merge defect rather than
	// a broken derivation.
	if len(written) == 0 {
		t.Fatal("the merge path's source yielded no written tables — the scan is broken, not the merge")
	}

	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	ctx := context.Background()

	rows, err := owner.Query(ctx, `
		SELECT c.conrelid::regclass::text AS src_table, a.attname AS src_col
		FROM pg_constraint c
		JOIN unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord) ON true
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
		WHERE c.contype = 'f'
		  AND c.confrelid::regclass::text = 'company'
		  AND a.attname <> 'workspace_id'
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		seen++
		if written[table] {
			continue
		}
		// Asked only once the column is already an offender: a waiver checked
		// before that would decay into a standing permission over every future
		// column in the same table.
		if companyFKsTheMergeLeaves.Waived(t, table+"."+column) {
			continue
		}
		t.Errorf("%s.%s points at company and the merge never writes %s.\n"+
			"A company merge retires one record into another, so every row naming the retired "+
			"one has to move, be dropped, or be declared. Left alone it points at a record no "+
			"read returns — nothing errors and the data is simply gone from the survivor's view, "+
			"until a hard delete of the archived company takes it for real. Move it in "+
			"internal/modules/contacts/mergerelink_company.go, or declare it in "+
			"companyFKsTheMergeLeaves with what moving it would cost.", table, column, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen < theCompanyFKFloor {
		t.Errorf("the census found %d company foreign keys, fewer than the %d floor: it is reading "+
			"a smaller schema than it thinks, and an under-counting census reports PASS over the "+
			"very columns it failed to look at", seen, theCompanyFKFloor)
	}
}

// tablesTheMergeWrites reads the merge path's own source and returns every
// table it writes.
//
// Source-scanned rather than listed, because a list would agree with itself
// forever: the point is to notice when a statement LEAVES the merge, and only
// the source can say that.
func tablesTheMergeWrites(t *testing.T) map[string]bool {
	t.Helper()
	written := map[string]bool{}
	for _, rel := range mergePathFiles {
		path := filepath.Join("..", rel)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the merge path %s: %v", rel, err)
		}
		for _, match := range sqlWriteTarget.FindAllStringSubmatch(string(body), -1) {
			written[strings.ToLower(match[1])] = true
		}
	}
	return written
}
