// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// The reference resolver's census, derived from the schema's own foreign keys.
//
// UNDER-RECOGNITION is the whole risk. A column the resolver cannot place is
// not a failure anywhere: referenceGuard returns "", the predicate compiles to
// a bare comparison, and the answer is a well-formed page that says which rows
// name a record the caller may not open. There is nothing to notice, which is
// exactly how `partner_company_id` stayed open while `company_id` beside it
// was closed — one is spelled for its target and the other for its role.
//
// So the expectation is read off the catalog rather than kept as a list beside
// the one it would be checking.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// searchableReference matches a catalog line declaring one table's foreign key:
// `public.<table>.<name> FOREIGN KEY (<column>) REFERENCES <target>(id)`.
var searchableReference = regexp.MustCompile(
	`^public\.(\w+)\.\w+ FOREIGN KEY \((\w+)\) REFERENCES (\w+)\(id\)`)

func TestEveryQueryableReferenceResolvesToItsRecordType(t *testing.T) {
	catalog, err := os.ReadFile("../../../migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}

	found := 0
	for _, line := range strings.Split(string(catalog), "\n") {
		m := searchableReference.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		table, column, target := m[1], m[2], m[3]
		// Both ends must be records this surface searches. A reference to a
		// pipeline or an app user names nothing a query plan can traverse, and
		// demanding a branch for it would fail on a column that discloses
		// nothing.
		if _, searchable := branchFor(table); !searchable {
			continue
		}
		if _, searchable := branchFor(target); !searchable {
			continue
		}
		found++

		branch, resolved := referencedBranch(table, Field{Name: column, Kind: KindID})
		if !resolved {
			t.Errorf("%s.%s references %s and the resolver places it nowhere, so a predicate on it compiles to a bare comparison — the row's presence then answers about a record the caller may not open, and nothing fails to say so",
				table, column, target)
			continue
		}
		if branch.entity != target {
			t.Errorf("%s.%s references %s but the resolver places it on %s — the guard would ask about the wrong record while looking present, which reads as a passing fix",
				table, column, target, branch.entity)
		}
	}
	if found == 0 {
		t.Fatal("the catalog declared no foreign key between two searchable records — the scan read the wrong file or the wrong shape, and an empty expectation passes against anything")
	}
}

// A declaration nothing needs is a claim the schema does not make. It costs
// nothing at runtime, but it is the half of the list that goes stale silently:
// a column renamed by a migration leaves its old spelling here, resolving a
// name no table has.
func TestEveryDeclaredRoleNameIsAColumnTheSchemaHolds(t *testing.T) {
	catalog, err := os.ReadFile("../../../migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	declared := map[string]bool{}
	for column := range roleNamedReferences {
		declared[column] = false
	}
	for column := range selfReferences {
		declared[column] = false
	}
	for _, line := range strings.Split(string(catalog), "\n") {
		m := searchableReference.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if _, held := declared[m[2]]; held {
			declared[m[2]] = true
		}
	}
	for column, held := range declared {
		if !held {
			t.Errorf("the resolver declares %q, which the schema holds no foreign key for — it resolves a name no table has", column)
		}
	}
}
