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
	"internal/modules/contacts/mergecompanycollisions.go",
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
var companyFKsTheMergeLeaves = gatekit.Waive(map[string]string{
	"suggestion_dismissal.company_id": "a dismissal is keyed by a fingerprint computed over the company's OWN id, so a row moved onto the survivor would hash differently from anything the survivor is ever offered and could never match again. The merge retires them with the company instead of moving rows that cannot work; a reader may be offered the equivalent suggestion about the survivor once, and dismissing it again sticks. Re-deriving the fingerprints belongs to the suggestion engine that defines them",
})

// sqlWriteTarget matches the table a statement writes.
var sqlWriteTarget = regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+([a-z_][a-z0-9_]*)`)

// assignsToSurvivor matches a column assigned the survivor's id, in the three
// spellings the merge path uses: a plain `SET company_id = $2`, the
// storekit.SQLf form `SET company_id=$%d`, and a column name concatenated into
// the statement (`SET `+column+` = $2`, which relinkLinkRows uses to serve both
// record types from one statement).
//
// Matching an assignment and not merely the table name is what stops the census
// certifying a merge that only DELETES. A relink whose UPDATE is removed leaves
// its `DELETE FROM x WHERE company_id = $1` behind, and a census reading table
// names alone reports that table covered while the merge destroys every row the
// retired company held.
var assignsToSurvivor = regexp.MustCompile(`(?i)([a-z_][a-z0-9_]*)\s*=\s*\$(?:2|%d)`)

// carriesSurvivorIntoInsert matches the INSERT … SELECT $2 form, where the
// survivor's id arrives positionally as the first selected value rather than as
// an assignment. The column it lands in is the first name in the INSERT's
// column list.
var carriesSurvivorIntoInsert = regexp.MustCompile(`(?is)INSERT\s+INTO\s+[a-z_][a-z0-9_]*\s*\(\s*([a-z_][a-z0-9_]*).*?SELECT\s+\$2`)

// concatenatedColumn matches a Go string concatenation standing where a column
// name belongs, so a statement that assembles its column still counts as
// covering the company column it is given.
var concatenatedColumn = regexp.MustCompile(`SET\s+` + "`" + `\s*\+\s*[a-z]`)

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
		if written[table+"."+column] {
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

// tablesTheMergeWrites reads the merge path's own source and returns the
// "table.column" pairs it MOVES onto the survivor.
//
// Source-scanned rather than listed, because a list would agree with itself
// forever: the point is to notice when a statement LEAVES the merge, and only
// the source can say that.
//
// A statement counts only when it both names the table and assigns some column
// to the survivor parameter. Table name alone was the first shape of this
// census and it was too weak in the direction that matters: a relink reduced to
// its DELETE still mentioned the table, so the census certified a merge that
// destroyed the rows instead of moving them.
func tablesTheMergeWrites(t *testing.T) map[string]bool {
	t.Helper()
	written := map[string]bool{}
	for _, rel := range mergePathFiles {
		path := filepath.Join("..", rel)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the merge path %s: %v", rel, err)
		}
		for _, stmt := range splitSQLStatements(string(body)) {
			target := sqlWriteTarget.FindStringSubmatch(stmt)
			if target == nil {
				continue
			}
			table := strings.ToLower(target[1])
			for _, moved := range assignsToSurvivor.FindAllStringSubmatch(stmt, -1) {
				written[table+"."+strings.ToLower(moved[1])] = true
			}
			if insert := carriesSurvivorIntoInsert.FindStringSubmatch(stmt); insert != nil {
				written[table+"."+strings.ToLower(insert[1])] = true
			}
			// A statement whose column name is concatenated in cannot say
			// WHICH column it moves, so it covers the company pointer of the
			// table it names — that being the only company column such a
			// statement is ever given.
			if concatenatedColumn.MatchString(stmt) {
				written[table+".company_id"] = true
			}
		}
	}
	return written
}

// splitSQLStatements cuts a Go source file into the individual SQL statements
// its string literals hold, so a column assignment is credited to the statement
// that actually contains it rather than to whatever table was named last in the
// file.
func splitSQLStatements(body string) []string {
	chunks := sqlStatementStart.Split(body, -1)
	// Split drops the delimiter, so re-attach it: each chunk after the first
	// begins where a statement keyword was found.
	for i, keyword := range sqlStatementStart.FindAllString(body, -1) {
		if i+1 < len(chunks) {
			chunks[i+1] = keyword + chunks[i+1]
		}
	}
	return chunks
}

// sqlStatementStart marks where one SQL statement begins in a Go source file.
var sqlStatementStart = regexp.MustCompile(`(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+`)
