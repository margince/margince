// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// The module catalog's Owns-tables column is the ownership map.
//
// docs/reference/modules.md says the column IS "the ownership declared in each
// module's doc.go and enforced by backend/gates/tableownership_test.go". For ten
// of its rows it was not: roughly thirty-five owned tables sat in the map and in
// no cell, and one cell named a table the map does not.
//
// UNDER-RECOGNITION READS AS COMPLETE, which is why a short cell is worse than
// an absent column. A reader placing a change opens the catalog, sees a module's
// tables listed, and has no way to tell the list is partial — so "which module
// owns this table" is answered wrong, and the answer is a boundary violation
// that tableownership_test.go then refuses at a point where the reason is much
// less obvious.
//
// Both directions, because they fail differently. A table in the map and not the
// cell is the short list above. A table in the cell and not the map is a claim
// about ownership nothing enforces — the reader is told a module owns something
// it may write nowhere.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// catalogModuleRow reads a catalog row: the bolded module name, then the cells.
// Anchored on the bold, which is what distinguishes a module row from the other
// tables on the page.
var catalogModuleRow = regexp.MustCompile(`^\|\s*\*\*([a-z0-9_]+)\*\*\s*\|`)

// backticked matches a code span. The Owns-tables cell carries prose alongside
// its list in four rows, and prose is not backticked — so reading the spans
// rather than splitting the cell is what keeps a parenthetical out of the table
// set.
var backticked = regexp.MustCompile("`([^`]*)`")

// tableName is what a name in that cell must look like to be one. It drops the
// route fragments and prose words a span may still carry.
var tableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ownsTablesColumn is the cell this gate reads, counting from the row's first.
// Named rather than found by header text because the header row is markdown
// like any other and a gate that searched it would be reading the same prose it
// is checking.
const ownsTablesColumn = 3

// modulePrefix is what a tableOwners value looks like when the owner IS a
// module. Everything else in that map is a compose package with a table of its
// own, which this page does not catalogue.
const catalogModulePrefix = "internal/modules/"

func TestTheModuleCatalogsOwnedTablesAreTheOwnershipMap(t *testing.T) {
	t.Parallel()
	page, err := os.ReadFile(moduleCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", moduleCatalog, err)
	}

	// MODULES only. tableOwners also names compose subpackages that own a table
	// of their own — a card's cache, a brief's rows — and this page is the
	// module catalog: a compose package is not a module and has no row here by
	// design. Judging them would report the page for not listing something it
	// does not claim to.
	declared := map[string]map[string]bool{}
	for table, owner := range tableOwners {
		module, isModule := strings.CutPrefix(owner, catalogModulePrefix)
		if !isModule {
			continue
		}
		if declared[module] == nil {
			declared[module] = map[string]bool{}
		}
		declared[module][table] = true
	}

	documented := map[string]map[string]bool{}
	for _, line := range strings.Split(string(page), "\n") {
		match := catalogModuleRow.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) <= ownsTablesColumn {
			t.Errorf("the row for %s has %d cells, so this gate cannot reach its Owns-tables column — "+
				"the table shape has changed", match[1], len(cells))
			continue
		}
		documented[match[1]] = tablesNamedIn(cells[ownsTablesColumn])
	}
	if len(documented) == 0 {
		t.Fatalf("no module rows parsed out of %s — an empty census passes exactly like a correct page",
			moduleCatalog)
	}

	// The UNION, not the catalog's own rows. Iterating what the page lists
	// asks the page to nominate its own subjects, so a module with tables in
	// the map and no row at all is never visited — which is the
	// under-recognition this gate exists to catch, in its most complete form:
	// a reader cannot discover the owner of those tables from the catalog at
	// all, and nothing says so.
	for _, module := range unionOf(documented, declared) {
		owned, mapped := declared[module]
		if !mapped {
			// A module that owns no table. Not a finding on its own — engine
			// modules that write through a sibling's store are a real shape
			// here — but a row CLAIMING tables is, because nothing enforces it.
			if claimed := namesOnlyIn(documented[module], nil); len(claimed) > 0 {
				t.Errorf("the %s row names %v and the ownership map gives that module no table at all — "+
					"a claim about ownership nothing enforces", module, claimed)
			}
			continue
		}
		if _, listed := documented[module]; !listed {
			t.Errorf("%s owns %v and %s has no row for it. The catalog is where a reader looks up which "+
				"module owns a table, so a module missing from it is one whose tables have no discoverable "+
				"owner", module, namesOnlyIn(owned, nil), moduleCatalog)
			continue
		}
		if len(documented[module]) == 0 {
			t.Errorf("the %s row names no table, and the map gives it %d — a cell whose format changed "+
				"reads as a module that owns nothing", module, len(owned))
			continue
		}
		if missing := namesOnlyIn(owned, documented[module]); len(missing) > 0 {
			t.Errorf("%s owns %v and the %s row does not name them. A reader placing a change sees the "+
				"cell and has no way to tell it is short, so they answer \"which module owns this\" wrong",
				module, missing, moduleCatalog)
		}
		if extra := namesOnlyIn(documented[module], owned); len(extra) > 0 {
			t.Errorf("the %s row names %v and the ownership map does not — a claim about ownership that "+
				"nothing enforces, so the reader is told a module owns what it may write nowhere",
				module, extra)
		}
	}
}

// tablesNamedIn reads the table names out of one Owns-tables cell.
func tablesNamedIn(cell string) map[string]bool {
	out := map[string]bool{}
	for _, span := range backticked.FindAllStringSubmatch(cell, -1) {
		for _, name := range strings.Split(span[1], ",") {
			name = strings.TrimSpace(name)
			if tableName.MatchString(name) {
				out[name] = true
			}
		}
	}
	return out
}

// namesOnlyIn is the names in one set and not the other, sorted.
func namesOnlyIn(in, other map[string]bool) []string {
	var out []string
	for name := range in {
		if !other[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// unionOf merges the module names both sides carry, sorted. Both sides are
// read because the two absences fail differently and neither is visible from
// the other's list.
func unionOf(documented, declared map[string]map[string]bool) []string {
	seen := map[string]bool{}
	for module := range documented {
		seen[module] = true
	}
	for module := range declared {
		seen[module] = true
	}
	out := make([]string, 0, len(seen))
	for module := range seen {
		out = append(out, module)
	}
	sort.Strings(out)
	return out
}
