// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A report dimension over a REFERENCE column hands its ids back in the
// aggregate. The engine's own gate covers the report's entity and says nothing
// about the records those rows point at, while a normal read of the same row
// masks exactly those references when the caller cannot open them
// (deals/fieldmask.go). So every reference dimension has to declare the table
// it points at, and this derives that obligation from the catalog rather than
// keeping a second list of which ones were remembered.

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// referenceColumns are the row-scoped records a report's rows can point AT,
// mapped to the table each names. A column here that a spec exposes as a
// dimension without declaring its scope is the finding.
//
// gatekit:fixture the reference columns a report row can carry, and the table each points at
var referenceColumns = map[string]string{
	colOrganizationID:     tableOrganization,
	colPartnerOrgID:       tableOrganization,
	colProjectID:          tableProject,
	activityProjectIDExpr: tableProject,
}

func TestEveryReferenceDimensionDeclaresItsScope(t *testing.T) {
	for name, spec := range prebuiltReports {
		for field, expr := range spec.dimensions {
			table, isReference := referenceColumns[expr]
			if !isReference {
				continue
			}
			declared, ok := spec.referenceScopes[expr]
			if !ok {
				t.Errorf("%s groups by %q, which names a row in %s the caller may not be able to open, "+
					"but declares no referenceScopes entry — the aggregate would report an id "+
					"this caller's own read of the same row masks",
					name, field, table)
				continue
			}
			if declared != table {
				t.Errorf("%s scopes %q against %q, want %q", name, field, declared, table)
			}
		}
	}
}

// A FILTER over a reference column is defended, by one of the two mechanisms
// that can defend it.
//
// The dimension case is obvious: the id is printed. A filter discloses the same
// fact through the answer's SHAPE — `count deals where organization_id =
// <guess>` returns 1 for a company that exists with a deal and 0 for one that
// does not, so a caller who cannot open that company learns it is there, one
// guess at a time.
//
// Two defences, and either suffices:
//
//   - filterScopes refuses the VALUE. requireFilterScopes runs auth.Require and
//     auth.EnsureVisibleLive on the id the caller sent, so an unreadable one is
//     404 before any row is counted. This is the stronger of the two — the
//     question is refused rather than answered narrowly — and it is what
//     project_id carries on every spec that offers it.
//   - referenceScopes narrows the ROWS. The filter binds, but only rows whose
//     referenced record the caller can open are counted, so the answer is 0
//     for a company they cannot see.
//
// A spec offering the filter with neither is the finding. deals-by-stage and
// pipeline-current offered organization_id with neither, and a filtered count
// there confirmed a capture-private company one guess at a time.
func TestEveryReferenceFilterIsDefended(t *testing.T) {
	judged := 0
	for name, spec := range prebuiltReports {
		for field, expr := range spec.filters {
			table, isReference := referenceColumns[expr]
			if !isReference {
				continue
			}
			judged++
			if _, refused := spec.filterScopes[field]; refused {
				continue
			}
			declared, narrowed := spec.referenceScopes[expr]
			if !narrowed {
				t.Errorf("%s filters by %q, which selects rows by a %s the caller may not be "+
					"able to open, and declares neither a filterScopes entry to refuse the "+
					"value nor a referenceScopes entry to narrow the rows — the count then "+
					"answers whether that record exists",
					name, field, table)
				continue
			}
			if declared != table {
				t.Errorf("%s scopes filter %q against %q, want %q", name, field, declared, table)
			}
		}
	}
	if judged == 0 {
		t.Fatal("no reference filters were read; this gate proves nothing about the specs it means to cover")
	}
}

// The ad-hoc vocabulary knows every reference the prebuilt catalog does.
//
// runAdHocPlan builds a spec from the schema descriptors, so every declared
// field becomes a dimension and a filter with no author to declare a scope for
// it. adHocReferenceTables is what supplies them, and a second list of the same
// facts is two answers to one question — this keeps them one.
//
// The two are keyed differently on purpose: the catalog by SQL expression,
// because a spec writes its own; the ad-hoc map by field name, because that
// vocabulary renders `t.` + name. So the comparison is over the column name
// both spellings end in.
func TestTheAdHocVocabularyScopesEveryReferenceTheCatalogDoes(t *testing.T) {
	judged := 0
	for column, table := range referenceColumns {
		// Only the plain columns: a reference the catalog reads through a
		// subselect (the activity's project) is not a descriptor field, and the
		// ad-hoc vocabulary cannot name it.
		name, isPlainColumn := strings.CutPrefix(column, "t.")
		if !isPlainColumn {
			continue
		}
		judged++
		declared, ok := adHocReferenceTables[name]
		if !ok {
			t.Errorf("the catalog scopes %s against %s, and adHocReferenceTables does not name it — "+
				"the datasource seam would group by it and hand back an id the caller's own read masks",
				column, table)
			continue
		}
		if declared != table {
			t.Errorf("adHocReferenceTables scopes %q against %q, want %q", name, declared, table)
		}
	}
	if judged == 0 {
		t.Fatal("no plain reference columns were read; this gate proves nothing")
	}
}

// A reference belongs in a spec's dimensions, never its measures.
//
// A measure is a number to aggregate, and the drill-through selects every
// measure onto its rows beside every dimension. A reference exposed as one
// would be handed back as an id by a path built to render sums — masked today
// only because maskedDerivationSelects looks measures up too, and countable
// nowhere as an aggregate: count over a partner id counts rows, sum over one
// is meaningless. So the honest place for it is a dimension, and this refuses
// the other spelling rather than leaving a spec free to choose the one whose
// only protection is a lookup somebody could simplify away.
func TestNoMeasureCarriesAReference(t *testing.T) {
	judged := 0
	for name, spec := range prebuiltReports {
		for field, expr := range spec.measures {
			judged++
			if table, isReference := referenceColumns[expr]; isReference {
				t.Errorf("%s measures %q, which names a row in %s rather than a number — "+
					"a reference is a dimension, and aggregating one answers no question",
					name, field, table)
			}
		}
	}
	if judged == 0 {
		t.Fatal("no measures were read; this gate proves nothing about the specs it means to cover")
	}
}

// The reverse direction: a declared scope must name a table the row-scope
// helpers actually know, or the clause it renders would fail at query time on
// a path no unit test reaches.
func TestEveryDeclaredReferenceScopeNamesARowScopedTable(t *testing.T) {
	for name, spec := range prebuiltReports {
		for column, table := range spec.referenceScopes {
			// A reference is a column of the row (`t.x`) or a scalar read off
			// the row (`... WHERE al.activity_id = t.id`); either way it must
			// name the report's own row alias, or the clause binds nothing.
			if !strings.Contains(column, "t.") {
				t.Errorf("%s scopes %q, which does not read the report's own row", name, column)
			}
			if table == "" {
				t.Errorf("%s scopes %q against no table", name, column)
			}
		}
	}
}

// Every reference a caller can SELECT BY resolves through the vocabulary
// namedByReport reads, so that naming one applies its scope.
//
// namedByReport (reportwhere.go) looks a group-by key up in spec.dimensions and
// a filter key in spec.filters. Those are the two maps whose values are SQL
// expressions comparable against spec.referenceScopes. This asserts the
// converse: every scoped reference IS reachable that way, so a spec cannot
// declare a scope for a column no caller can name — a scope that never renders
// reads exactly like one that does.
//
// It says nothing about spec.thresholds, and that is the point of writing it
// this way rather than scanning threshold SQL for column names: a threshold
// renders its own clause from a bind position, so a reference inside one would
// be spelled however its author spelled it, and a substring scan would miss the
// case it exists to catch. What holds instead is that a threshold takes a
// NUMBER from the caller (thresholdValue) and compares it — it cannot carry a
// caller-supplied reference id at all, so there is nothing for a scope to gate.
func TestEveryScopedReferenceIsReachableThroughTheVocabulary(t *testing.T) {
	judged := 0
	for name, spec := range prebuiltReports {
		for column := range spec.referenceScopes {
			judged++
			reachable := slices.Contains(slices.Collect(maps.Values(spec.dimensions)), column) ||
				slices.Contains(slices.Collect(maps.Values(spec.filters)), column)
			if !reachable {
				t.Errorf("%s scopes %s, which is neither a dimension nor a filter — "+
					"namedByReport resolves a caller's key through those two maps, so this "+
					"scope renders for no question anybody can ask",
					name, column)
			}
		}
	}
	// A census that reads no references would pass over nothing at all.
	if judged == 0 {
		t.Fatal("no reference scopes were read; this gate proves nothing about the specs it means to cover")
	}
}
