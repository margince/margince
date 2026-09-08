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
	"context"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// referenceColumns are the row-scoped records a report's rows can point AT,
// mapped to the table each names. A column here that a spec exposes as a
// dimension without declaring its scope is the finding.
//
// gatekit:fixture the reference columns a report row can carry, and the table each points at
var referenceColumns = map[string]string{
	colCompanyID:          tableCompany,
	colPartnerCompanyID:   tableCompany,
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
// fact through the answer's SHAPE — `count deals where company_id =
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
// pipeline-current offered company_id with neither, and a filtered count
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

// A dimension or filter read over a JOIN declares where its row scope comes
// from.
//
// referenceColumns above is a list of id expressions, all `t.`-prefixed, and it
// answers only for columns on the report's own table. A joined attribute —
// `company.size_band`, and whatever `company.industry` somebody adds next — is not an
// id and appears in no such list, so both gates above skip it and it can ship
// carrying no row scope at all. That is the exact defect scopeVia was added to
// fix, and a point fix without a gate invites its second instance.
//
// Derived from the spec's own joins rather than a list beside them: the alias
// each join introduces is what marks an expression as coming from another
// table, so a spec that adds a join is covered the moment it does.
//
// Only a join onto a ROW-SCOPED table is asked for a scopeVia. The forecast
// joins `stage` for win_probability, and a stage is pipeline configuration
// every seat may read — there is no row scope for it to inherit, and demanding
// one would be a false positive the next author has to argue with. Which
// tables those are is read from auth.ScopeClauseFor rather than listed here,
// so a table gaining or losing its row scope moves this gate with it.
func TestEveryJoinedVocabularyDeclaresWhereItsScopeComesFrom(t *testing.T) {
	t.Parallel()
	checked := 0
	for _, report := range slices.Sorted(maps.Keys(prebuiltReports)) {
		spec := prebuiltReports[report]
		aliases := joinAliases(spec)
		if len(aliases) == 0 {
			continue
		}
		for _, vocabulary := range []map[string]string{spec.dimensions, spec.filters} {
			for _, field := range slices.Sorted(maps.Keys(vocabulary)) {
				expr := vocabulary[field]
				alias, joined := joinedAlias(expr, aliases)
				if !joined || !rowScopedTable(t, aliases[alias]) {
					continue
				}
				checked++
				if _, declared := spec.scopeVia[field]; declared {
					continue
				}
				t.Errorf("report %q offers %q, which reads row-scoped %q over a join, and "+
					"declares no scopeVia.\n\n"+
					"referenceScopes renders `ref.id = <column>` and reaches only a column "+
					"that IS an id, so a joined attribute matches nothing and carries NO "+
					"row scope: a seat excluded from the joined record can group by its "+
					"attributes and read the counts off the rows. Name the id column this "+
					"inherits from in scopeVia.", report, field, aliases[alias])
			}
		}
	}
	// A census that can fail short has already failed. If no spec joins a
	// row-scoped table any more, or the alias parse stops matching, this walks
	// nothing and says so rather than reporting a green it did not earn.
	if checked == 0 {
		t.Fatal("no report exposes a vocabulary entry read over a join onto a row-scoped " +
			"table — either the catalog changed shape or joinAliases stopped parsing, " +
			"and this gate is holding nothing")
	}
}

// A vocabulary entry read over a join declares SOME defence — a grant or a
// scopeVia — for the table it reaches into.
//
// The gate above asks only about ROW scope, and answers "nothing owed" for a
// joined table that has none. That is right for `stage`, which is pipeline
// configuration every seat reads. It is wrong for a table whose protection is
// an OBJECT grant instead, and the difference is invisible from the row-scope
// registry: sdr_handoff carries no row scope and is gated by auth.Require on
// `lead`, so the row-scope gate passed it while a seat holding activity.read
// alone read handoff acceptances off a meeting report.
//
// This gate cannot say WHICH object a table owes — nothing in the tree maps a
// table to its RBAC object, and inventing that map here would be a second copy
// of a fact the owning module states. What it can say is that somebody decided:
// a joined table is either configuration everyone may read, in which case it is
// ratified below by name, or it is protected, in which case the vocabulary
// entry names its grant or its scopeVia. Silence is what shipped the leak.
func TestEveryJoinedVocabularyDeclaresADefence(t *testing.T) {
	t.Parallel()
	defer freelyReadableJoins.AssertAllMatched(t)

	checked := 0
	for _, report := range slices.Sorted(maps.Keys(prebuiltReports)) {
		spec := prebuiltReports[report]
		aliases := joinAliases(spec)
		if len(aliases) == 0 {
			continue
		}
		for _, vocabulary := range []map[string]string{spec.dimensions, spec.filters} {
			for _, field := range slices.Sorted(maps.Keys(vocabulary)) {
				alias, joined := joinedAlias(vocabulary[field], aliases)
				if !joined {
					continue
				}
				checked++
				if _, granted := spec.grants[field]; granted {
					continue
				}
				if _, via := spec.scopeVia[field]; via {
					continue
				}
				if freelyReadableJoins.Waived(t, aliases[alias]) {
					continue
				}
				t.Errorf("report %q offers %q, which reads joined table %q, and declares "+
					"neither a grant nor a scopeVia.\n\n"+
					"A joined table's own gate does not travel with the join: this report's "+
					"object check asks about %q and nothing asks about %q. Name the object in "+
					"spec.grants, or the id column its row scope inherits from in "+
					"spec.scopeVia, or ratify the table in freelyReadableJoins if every seat "+
					"may read it.",
					report, field, aliases[alias], spec.entity, aliases[alias])
			}
		}
	}
	if checked == 0 {
		t.Fatal("no report exposes a vocabulary entry read over a join — either the catalog " +
			"changed shape or joinAliases stopped parsing, and this gate is holding nothing")
	}
}

// freelyReadableJoins are the joined tables no seat is kept out of, so a
// vocabulary entry reading one owes neither a grant nor a row scope.
//
// By TABLE rather than by field: what makes a stage freely readable is a fact
// about stages, and a waiver written per field would need restating for every
// vocabulary entry that reads one.
var freelyReadableJoins = gatekit.Waive(map[string]string{
	"stage": "pipeline configuration. A stage's name and win_probability are the board every " +
		"seat already works on, and the forecast reads win_probability to weight a deal the " +
		"caller has passed the deal gate for — there is no seat that may see the deal and not " +
		"the stage it sits in",
})

// rowScopedTable asks the row-scope machinery itself whether a table has a
// scope, rather than keeping a second copy of its registry.
//
// ScopeClauseFor refuses a table it does not scope with a distinct error and
// answers every other question about the CALLER, so a background context is
// enough to tell the two apart: a non-scoped table errors, and a scoped one
// gets as far as needing an actor.
func rowScopedTable(t *testing.T, table string) bool {
	t.Helper()
	_, err := auth.ScopeClauseFor(context.Background(), table, "ref", func(any) int { return 1 })
	return err == nil || !strings.Contains(err.Error(), "not a row-scoped table")
}

// joinAliases maps every alias a spec's joins introduce to the table it names.
//
// TWO shapes, and the second is why this does not stop at the first match.
//
//   - `[LEFT ]JOIN <table> <alias> ON …`, the plain join.
//   - `LEFT JOIN LATERAL ( … ) <alias> ON …`, whose alias names a SUBQUERY. The
//     table a caller of that alias really reaches is the one in the subquery's
//     FROM, and the subquery may carry plain joins of its own.
//
// Reading only the first match is what made this gate blind. Against a lateral
// it matched the subquery's INNER join and returned that alias, so the outer
// alias — the one every exposed expression is written against — never entered
// the map, joinedAlias answered false for it, and the dimension was skipped
// without being checked. The gate reported PASS having looked at nothing, which
// is the failure AGENTS.md rule 8 names: a census that fails short reads
// exactly like one that passes.
func joinAliases(spec reportSpec) map[string]string {
	aliases := map[string]string{}
	for _, join := range spec.joins {
		for _, m := range joinAlias.FindAllStringSubmatch(join, -1) {
			aliases[m[2]] = m[1]
		}
		// The lateral's alias sits after `)` rather than after a table name, so
		// it needs its own pattern, and the table it stands for comes from the
		// subquery's FROM.
		alias := lateralAlias.FindStringSubmatch(join)
		from := lateralFrom.FindStringSubmatch(join)
		if alias != nil && from != nil {
			aliases[alias[1]] = from[1]
		}
	}
	return aliases
}

var (
	joinAlias    = regexp.MustCompile(`JOIN\s+(\w+)\s+(\w+)\s+ON\b`)
	lateralAlias = regexp.MustCompile(`\)\s*(\w+)\s+ON\b`)
	lateralFrom  = regexp.MustCompile(`FROM\s+(\w+)\s`)
)

// joinedAlias reports which joined alias an expression reads, if any.
//
// Matches on `alias.`, so `company.size_band` reads the `company` join and
// `t.company_id` matches nothing — the base table is not a join.
func joinedAlias(expr string, aliases map[string]string) (string, bool) {
	for alias := range aliases {
		if strings.Contains(expr, alias+".") {
			return alias, true
		}
	}
	return "", false
}
