// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A name that reads another record type is defended on EVERY surface that can
// ask for it — grouped by, aggregated over, and filtered on.
//
// The engine's vocabulary gate reads the names a caller spelled. It read the
// group-by and the measures and not the filters, so "how many deals at this
// project" answered a question "group by project" was refused for: the count
// is the disclosure, because a non-zero one says the record exists and has work
// under it.
//
// The corpus is the catalog itself rather than a list beside it. A gate that
// names its own subjects stops covering the spec added after it was written,
// and the defect it guards is exactly the kind a new spec reintroduces.

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
)

// runSpec passes the caller's FILTER keys to the vocabulary gate.
//
// This is the assertion that fails when the fix is reverted, and it is written
// against the call site rather than against a served request because the
// catalog cannot currently present the failing case: every granted filter it
// offers today is project_id, which also carries filterScopes — the stronger
// defence, refusing the VALUE before a row is counted. So an end-to-end test
// over a shipped report is green either way and proves nothing.
//
// What the vocabulary gate adds is the answer for a granted filter that has no
// row scope to check, which is the spec somebody writes next.
func TestTheVocabularyGateReadsTheCallersFilterKeys(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("report.go")
	if err != nil {
		t.Fatalf("reading the engine: %v", err)
	}
	// The names collected before withThresholdDefaults, and the call that reads
	// them. Both halves: collecting the keys and never passing them is the
	// shape the defect had.
	for _, needed := range []string{
		"filtered := slices.Sorted(maps.Keys(req.Filters))",
		"slices.Concat(asked, filtered, aggregateFields(req.Aggregates))",
	} {
		if !strings.Contains(string(source), needed) {
			t.Errorf("runSpec no longer carries %q.\n\n"+
				"A filter is a read: grouping by a granted field and filtering on one "+
				"ask the same question, and the count is the answer. The keys must be "+
				"collected BEFORE withThresholdDefaults, so a threshold the spec "+
				"supplied is not refused to the caller who never asked for it.", needed)
		}
	}
}

// A filter that names another record type is defended THREE ways, and every
// such filter in the catalog carries at least one.
//
// They are not equivalent, and which one fits depends on what the filter is:
//
//   - filterScopes refuses the VALUE (auth.EnsureVisibleLive — an unreadable id
//     is 404 before a row is counted). The strongest, and the only one that
//     also answers a caller holding the object grant but not that row.
//   - referenceScopes narrows the ROWS to the referenced records the caller can
//     open, so a count filtered to a company they cannot see counts nothing.
//     This is what the deal reports use for company_id and partner_company_id.
//   - The vocabulary grant refuses the NAME, for a field whose record type the
//     caller may not read at all.
//
// The rule is at least one, not a particular one. Demanding filterScopes
// everywhere would be wrong: a filter over a record type with no row scope has
// nothing for it to check, and the deal reports' company filters are
// deliberately defended by the row scope instead.
func TestEveryFilterOverAScopedReferenceIsDefended(t *testing.T) {
	t.Parallel()
	checked := 0
	for _, report := range slices.Sorted(maps.Keys(prebuiltReports)) {
		spec := prebuiltReports[report]
		for _, field := range slices.Sorted(maps.Keys(spec.filters)) {
			expr := spec.filters[field]
			_, valueChecked := spec.filterScopes[field]
			_, nameChecked := spec.grants[field]
			table, rowScoped := spec.referenceScopes[expr]
			if !valueChecked && !nameChecked && !rowScoped {
				// Only reachable for a filter naming a column some OTHER spec
				// row-scopes, or an id-shaped filter added without any of the
				// three. Both are the disclosure this test exists for.
				if referencedElsewhere(field, expr) {
					t.Errorf("report %q filters on %q, which other specs treat as a "+
						"row-scoped reference, and none of filterScopes, grants or "+
						"referenceScopes defends it here: a count filtered to one "+
						"record answers whether that record exists", report, field)
				}
				continue
			}
			checked++
			if rowScoped && !valueChecked && !nameChecked && table == "" {
				t.Errorf("report %q scopes filter %q against an empty table name", report, field)
			}
		}
	}
	// A census that can fail short has already failed: if the walk stops seeing
	// defended filters, it passes over nothing and says so.
	if checked == 0 {
		t.Fatal("no report in the catalog offers a filter over another record type — " +
			"this gate is holding nothing, so either the catalog changed shape or the walk broke")
	}
}

// referencedElsewhere reports whether any spec row-scopes this expression.
//
// A column one report treats as a scoped reference and another leaves bare is
// the asymmetry worth failing on: the same disclosure, defended on one surface
// and not the next.
func referencedElsewhere(field, expr string) bool {
	for _, spec := range prebuiltReports {
		if _, ok := spec.referenceScopes[expr]; ok {
			return true
		}
		if _, ok := spec.filterScopes[field]; ok {
			return true
		}
	}
	return false
}

// A grants entry names something the report actually offers.
//
// An entry for a name that is not a dimension, measure or filter defends
// nothing, and reads as protection — which is worse than no entry at all,
// because the next author greps, finds it, and stops looking.
func TestEveryGrantedNameIsInTheReportsVocabulary(t *testing.T) {
	t.Parallel()
	for _, report := range slices.Sorted(maps.Keys(prebuiltReports)) {
		spec := prebuiltReports[report]
		for _, field := range slices.Sorted(maps.Keys(spec.grants)) {
			_, isDimension := spec.dimensions[field]
			_, isMeasure := spec.measures[field]
			_, isFilter := spec.filters[field]
			if !isDimension && !isMeasure && !isFilter {
				t.Errorf("report %q owes a grant for %q, which is not one of its "+
					"dimensions, measures or filters: the entry defends nothing and "+
					"reads as protection the next author will not re-derive", report, field)
			}
		}
	}
}
