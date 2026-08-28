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

func TestTheModuleCatalogsOwnedTablesAreTheOwnershipMap(t *testing.T) {
	t.Parallel()
	page, err := os.ReadFile(moduleCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", moduleCatalog, err)
	}

	declared := map[string]map[string]bool{}
	for table, owner := range tableOwners {
		module := strings.TrimPrefix(owner, "internal/modules/")
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

	for _, module := range sortedModules(documented) {
		owned, mapped := declared[module]
		if !mapped {
			// A module that owns no table at all. Nothing to compare, and not a
			// finding: engine modules that write through a sibling's store are
			// a real shape here.
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

func sortedModules(in map[string]map[string]bool) []string {
	out := make([]string, 0, len(in))
	for module := range in {
		out = append(out, module)
	}
	sort.Strings(out)
	return out
}
