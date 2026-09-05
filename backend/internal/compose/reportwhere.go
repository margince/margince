// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The WHERE side of a report: what the spec asserts, what the caller asked
// for, which rows the caller may READ, and which population the answer is
// about. Four different questions, and the file exists so they are visibly
// four rather than one long predicate.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// buildReportWhere assembles the WHERE side — the spec's base predicate,
// the validated caller filters (sorted for a deterministic plan echo), and
// the caller's row-scope clause — binding every value through arg.
func buildReportWhere(
	ctx context.Context, tx pgx.Tx, spec reportSpec, req reportRequest,
	requested RequestedScope, arg func(any) int,
) ([]string, error) {
	// A spec may restrict nothing: leads-by-status counts every lead whatever
	// its status, because the archived ones ARE its subject. An empty base
	// joined in with the rest would render `WHERE  AND …`, so the absence is
	// dropped here rather than paid for with a `TRUE` in every such spec — a
	// spec should not have to spell a no-op to say it has no restriction.
	var where []string
	if spec.baseWhere != "" {
		where = append(where, spec.baseWhere)
	}
	// Deterministic filter order — the plan echo and the SQL must not
	// depend on map iteration.
	filterKeys := make([]string, 0, len(req.Filters))
	for key := range req.Filters {
		filterKeys = append(filterKeys, key)
	}
	sort.Strings(filterKeys)
	for _, key := range filterKeys {
		if threshold, ok := spec.thresholds[key]; ok {
			n, err := thresholdValue(key, req.Filters[key])
			if err != nil {
				return nil, err
			}
			where = append(where, threshold.clause(arg(n)))
			continue
		}
		expr, ok := spec.filters[key]
		if !ok {
			// catalogFilterNames, the same helper the tool catalog renders from:
			// `filters` is ONE object holding two families, and a refusal built
			// from the equality filters alone omits the thresholds the loop above
			// just accepted. A caller who misspells a threshold would read a list
			// without that family in it and conclude the report cannot answer
			// their question.
			return nil, &FieldNotAllowedError{Field: key, Slot: slotFilters, Allowed: catalogFilterNames(spec)}
		}
		// A null filter means "not set", the SAME meaning the drill-through
		// gives an empty group key (derivationWhere). Binding it as `= NULL`
		// instead — which is never true — let one response answer "no rows"
		// while the derivation handle it minted in the same breath answered
		// with every unset row. Two doors onto one question have to agree, or
		// the explanation disagrees with the number it explains.
		if req.Filters[key] == nil {
			where = append(where, expr+" IS NULL")
			continue
		}
		value, err := reportFilterValue(key, req.Filters[key])
		if err != nil {
			return nil, err
		}
		where = append(where, fmt.Sprintf("%s = $%d", expr, arg(value)))
	}
	scoped, err := specScopeClauses(ctx, spec, arg)
	if err != nil {
		return nil, err
	}
	where = append(where, scoped...)
	// WHICH population, as against which rows the caller may read at all.
	//
	// Row scope does not answer it: a deal is an identity table read by every
	// seat, so the clause above renders TRUE and a rep's Pipeline showed the
	// whole installation while their Forecast — narrowed by this same resolver
	// since #4077 — showed their own. Two Analytics tabs disagreeing about
	// which records they cover, with nothing on screen saying so.
	if spec.population == measureCallersOwn {
		population, err := reportPopulationClause(ctx, tx, requested, arg)
		if err != nil {
			return nil, err
		}
		if population != "" {
			where = append(where, population)
		}
	}
	refs, err := referenceScopeClauses(ctx, spec, namedByReport(spec, req), arg)
	if err != nil {
		return nil, err
	}
	return append(where, refs...), nil
}

// specScopeClauses is the ROW-SCOPE half of a population's narrowing: the
// entity's own scope, or the activity content walk for a spec that reads
// message bodies. The masks and reference scopes are the other two halves;
// specNarrowings below composes all three.
//
// Extracted so the generic analytics path and the report engine ask ONE
// question. They did not, and the difference was not academic: the analytics
// path shipped without the activity content clause, which is what enforces
// `restricted_at`, the link-reachability walk and the audience a human set on
// a thread — and the audience arm does not yield to row_scope=all, so an admin
// read private mail through a count.
//
// Two writers of one invariant either share a helper or say why they do not.
// This is the helper.
func specScopeClauses(ctx context.Context, spec reportSpec, arg func(any) int) ([]string, error) {
	var scope string
	var err error
	if spec.activityWalk {
		scope, err = auth.ActivityContentClause(ctx, "t", arg)
	} else {
		scope, err = auth.ScopeClauseFor(ctx, string(spec.entity), "t", arg)
	}
	if err != nil {
		return nil, err
	}
	var out []string
	if scope != "" {
		out = append(out, scope)
	}
	return out, nil
}

// specNarrowings is EVERY row-level narrowing a spec's population carries: the
// scope clauses above, the field-mask exclusions, and the reference scopes.
//
// Held by: TestEveryPopulationsNarrowingsAreComposedInOnePlace
// (backend/gates/analyticsscope_test.go), which fails when a function taking a
// reportSpec reaches for one of the three directly instead.
//
// The report engine does not call this one. It needs the mask clauses in hand
// separately, because it also COUNTS the rows they withheld for
// excluded_by_permission — so it composes the three itself, and the gate
// TestEveryPopulationsNarrowingsAreComposedInOnePlace ratifies it by name.
// Every other path takes the whole set from here.
func specNarrowings(
	ctx context.Context, tx pgx.Tx, spec reportSpec, requested RequestedScope,
	named referencedColumns, arg func(any) int,
) ([]string, error) {
	out, err := specScopeClauses(ctx, spec, arg)
	if err != nil {
		return nil, err
	}
	// The POPULATION the caller asked to measure, resolved against their own
	// lens. Record authorization is not it: deals are workspace-readable by
	// design, so row scope alone answers an own-lens rep with the whole
	// installation's numbers — the question they asked was about their work.
	//
	// Here rather than at the call site, so the one composer this gate holds
	// carries every narrowing. A caller that resolved the population itself
	// and appended it would be the second partial answer the gate exists to
	// refuse, and it would pass, because the gate reads the clause builders
	// and not what a caller does after them.
	//
	// Gated on spec.population like buildReportWhere's own population half:
	// a spec that opts out (measureEveryReadableRow) means it, on every door
	// onto it — the saved-analytics path this composes for must not narrow a
	// report the run_report path already answers unnarrowed, and for a spec
	// with no owner_id column (activities-by-kind) resolving this
	// unconditionally is not a narrower answer, it is a crash.
	if spec.population == measureCallersOwn {
		_, population, err := AnalyticsPopulationClause(ctx, tx, requested, "t", arg)
		if err != nil {
			return nil, err
		}
		if population != "" {
			out = append(out, population)
		}
	}
	// An aggregate over a masked column would disclose it through the total,
	// so the row leaves the population entirely.
	masks, _, err := maskExclusionClauses(ctx, spec, arg)
	if err != nil {
		return nil, err
	}
	// And the records the population POINTS AT: grouping by a reference column
	// would otherwise name ids the caller's ordinary read of the same row masks.
	refs, err := referenceScopeClauses(ctx, spec, named, arg)
	if err != nil {
		return nil, err
	}
	return append(append(out, masks...), refs...), nil
}

// referencedColumns is the set of SQL expressions one query SELECTS BY — the
// columns it groups on or filters on, whose values therefore shape the answer
// whether or not any id is printed.
//
// Not the columns it merely returns: those are masked per row where a caller
// could reach one (maskedDerivationSelects), because blanking a column keeps
// the row in a population that both halves of an answer have to agree on.
//
// Keyed by the same expression string spec.referenceScopes uses, so a lookup is
// a comparison and not a second spelling of the mapping.
type referencedColumns map[string]bool

// namedByReport is what a prebuilt report exposes: the dimensions it groups by
// and the filters it was given.
//
// A filter counts even though it prints nothing. Filtering on a partner the
// caller cannot open would confirm which deals that partner brought — the
// answer's SHAPE discloses the reference even when its id never appears.
//
// A spec may instead defend a filter with filterScopes, which refuses the VALUE
// (auth.EnsureVisibleLive, so an unreadable id is 404 before a row is counted)
// and is the stronger of the two — project_id carries that on every spec that
// offers it, and declares no reference scope. Naming a column here that no
// referenceScopes entry covers renders nothing, so this set stays the same
// question for both families.
func namedByReport(spec reportSpec, req reportRequest) referencedColumns {
	named := make(referencedColumns, len(req.GroupBy)+len(req.Filters))
	for _, field := range req.GroupBy {
		if expr, ok := spec.dimensions[field]; ok {
			named[expr] = true
		}
	}
	for field := range req.Filters {
		if expr, ok := spec.filters[field]; ok {
			named[expr] = true
		}
	}
	return named
}

// namedByDerivation is what a drill-through SELECTS ON: the predicates the
// handle carries, whether they came from the headline's filters or from the
// group this cell belongs to.
//
// Its output columns are deliberately not here. A drill-through returns every
// dimension the spec declares, so taking the columns would scope every
// reference on every drill-through and put back the exclusion this change
// removes. maskedDerivationSelects (derivationfetch.go) covers the output side
// instead, by blanking the reference and keeping the row — which is what an
// ordinary read of the same record does.
//
// So the two halves divide the obligation: a reference a predicate SELECTS BY
// narrows the rows, and a reference a column merely RETURNS is masked. Both
// leave the drill-through reading the same population as its headline.
func namedByDerivation(spec reportSpec, plan derivationPlan) referencedColumns {
	named := make(referencedColumns, len(plan.preds)+len(plan.groupBy))
	for _, p := range plan.preds {
		named[p.expr] = true
	}
	// And what the HEADLINE grouped by, which the handle names even where it
	// pins no value. A result-level handle explains every cell of one answer at
	// once, so it binds no group key — but the answer it explains was still
	// narrowed by grouping over a reference, and a drill-through that forgot
	// that opened records the count above it never counted.
	for _, field := range plan.groupBy {
		if expr, ok := spec.dimensions[field]; ok {
			named[expr] = true
		}
	}
	return named
}

// referenceScopeClauses narrows a report to the rows whose REFERENCED records
// the caller could open, for every reference the query NAMES.
//
// The engine's own gate covers the report's entity and nothing it points at, so
// a dimension over a reference column would otherwise hand back ids that the
// same caller's ordinary read of the same row masks. Excluding the row is the
// only honest aggregate answer there: there is no per-row place to write
// "withheld", and folding those deals under a null key would still say that
// SOME partner brought them.
//
// `named` is what bounds it: the columns this query groups by, filters on or
// returns. A reference the answer never mentions discloses nothing, so
// excluding its rows buys no privacy and costs correctness — a stage total that
// drops the deals whose partner is capture-private reports less money than the
// same reader's own deal list shows them, for a reason nothing on screen
// states, and its drill-through link then opens more records than the count
// counted.
//
// The two callers therefore pass different sets, and the difference is the
// point. A report names its group-bys and its filters. A drill-through puts
// every dimension on its rows, so it names its output columns as well —
// an explanation must not out-see the number it explains.
//
// This is the ROW-SCOPE obligation only. A reference that also takes the
// referenced table's OBJECT grant before it may be named at all (the project
// an activity is filed under) declares that in spec.grants, and the
// vocabulary gate (reportgrants.go) refuses the plan by name; a row scope
// clause that renders empty means an unbounded reader of the table and
// nothing about the grant, which is why the two are asked separately.
// TestGroupingByPartnerDoesNotNameAPartnerTheCallerCannotOpen pins the
// partner dimension to the row scope alone.
func referenceScopeClauses(
	ctx context.Context, spec reportSpec, named referencedColumns, arg func(any) int,
) ([]string, error) {
	if len(spec.referenceScopes) == 0 {
		return nil, nil
	}
	clauses := make([]string, 0, len(spec.referenceScopes))
	for _, column := range slices.Sorted(maps.Keys(spec.referenceScopes)) {
		if !named[column] {
			continue
		}
		table := spec.referenceScopes[column]
		scope, err := auth.ScopeClauseFor(ctx, table, "ref", arg)
		if err != nil {
			return nil, err
		}
		if scope == "" {
			// An unbounded reader of that table: every row it could name is one
			// they may open, so there is nothing to narrow.
			continue
		}
		clauses = append(clauses, fmt.Sprintf(
			"(%s IS NULL OR EXISTS (SELECT 1 FROM %s ref WHERE ref.id = %s AND %s))",
			column, table, column, scope))
	}
	return clauses, nil
}
