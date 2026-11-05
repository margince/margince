// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// The module catalog's Owns-tables column links to the entity model, and
// copies no table.
//
// The entity model is rendered from tableOwners. A typed cell drifted: ten rows
// once missed about thirty-five tables. The census runs over the map's modules,
// so a module with tables and no row is found too.

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

// backticked matches a code span, which is where a copied table name would sit.
var backticked = regexp.MustCompile("`([^`]*)`")

// ownsTablesColumn is the cell this gate reads, counting from the row's first.
// Named rather than found by header text because the header row is markdown
// like any other and a gate that searched it would be reading the same prose it
// is checking.
const ownsTablesColumn = 3

// modulePrefix is what a tableOwners value looks like when the owner IS a
// module. Everything else in that map is a compose package with a table of its
// own, which this page does not catalogue.
const catalogModulePrefix = "internal/modules/"

func TestTheModuleCatalogsOwnedTablesLinkToTheEntityModel(t *testing.T) {
	t.Parallel()
	page, err := os.ReadFile(moduleCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", moduleCatalog, err)
	}

	// Modules only: a compose package is not a module and has no row here.
	owning := map[string]bool{}
	for _, owner := range tableOwners {
		if module, isModule := strings.CutPrefix(owner, catalogModulePrefix); isModule {
			owning[module] = true
		}
	}

	cellsByModule := map[string]string{}
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
		cellsByModule[match[1]] = cells[ownsTablesColumn]
	}
	if len(cellsByModule) == 0 {
		t.Fatalf("no module rows parsed out of %s — an empty census passes like a correct page", moduleCatalog)
	}

	for module := range owning {
		cell, listed := cellsByModule[module]
		if !listed {
			t.Errorf("%s owns tables and %s has no row for it, so a reader of the catalog cannot find "+
				"where its tables are listed", module, moduleCatalog)
			continue
		}
		if want := "](entity-model/" + module + ".md)"; !strings.Contains(cell, want) {
			t.Errorf("the %s row's Owns-tables cell does not link to entity-model/%s.md, the page "+
				"rendered from tableOwners", module, module)
		}
	}
	for module, cell := range cellsByModule {
		if copied := tablesNamedIn(cell); len(copied) > 0 {
			t.Errorf("the %s row names %v. The table list is entity-model/%s.md; a copy in the "+
				"catalog drifts from it", module, namesOnlyIn(copied, nil), module)
		}
	}
}

// tablesNamedIn reads the names in one cell's code spans that tableOwners
// knows as tables. A file name such as `tableownership_test.go` is not one.
func tablesNamedIn(cell string) map[string]bool {
	out := map[string]bool{}
	for _, span := range backticked.FindAllStringSubmatch(cell, -1) {
		for _, name := range strings.Split(span[1], ",") {
			name = strings.TrimSpace(name)
			if _, isTable := tableOwners[name]; isTable {
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
