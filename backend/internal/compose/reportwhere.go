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
	where := []string{spec.baseWhere}
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
	refs, err := referenceScopeClauses(ctx, spec, arg)
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
	ctx context.Context, tx pgx.Tx, spec reportSpec, requested RequestedScope, arg func(any) int,
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
	_, population, err := AnalyticsPopulationClause(ctx, tx, requested, "t", arg)
	if err != nil {
		return nil, err
	}
	if population != "" {
		out = append(out, population)
	}
	// An aggregate over a masked column would disclose it through the total,
	// so the row leaves the population entirely.
	masks, _, err := maskExclusionClauses(ctx, spec, arg)
	if err != nil {
		return nil, err
	}
	// And the records the population POINTS AT: grouping by a reference column
	// would otherwise name ids the caller's ordinary read of the same row masks.
	refs, err := referenceScopeClauses(ctx, spec, arg)
	if err != nil {
		return nil, err
	}
	return append(append(out, masks...), refs...), nil
}

// referenceScopeClauses narrows a report to the rows whose REFERENCED records
// the caller could open, for every reference the spec declares.
//
// The engine's own gate covers the report's entity and nothing it points at, so
// a dimension over a reference column would otherwise hand back ids that the
// same caller's ordinary read of the same row masks. Excluding the row is the
// only honest aggregate answer: there is no per-row place to write "withheld",
// and folding those deals under a null key would still say that SOME partner
// brought them.
//
// This is the ROW-SCOPE obligation only. A reference that also takes the
// referenced table's OBJECT grant before it may be named at all (the project
// an activity is filed under) declares that in spec.grants, and the
// vocabulary gate (reportgrants.go) refuses the plan by name; a row scope
// clause that renders empty means an unbounded reader of the table and
// nothing about the grant, which is why the two are asked separately.
// TestGroupingByPartnerDoesNotNameAPartnerTheCallerCannotOpen pins the
// partner dimension to the row scope alone.
func referenceScopeClauses(ctx context.Context, spec reportSpec, arg func(any) int) ([]string, error) {
	if len(spec.referenceScopes) == 0 {
		return nil, nil
	}
	clauses := make([]string, 0, len(spec.referenceScopes))
	for _, column := range slices.Sorted(maps.Keys(spec.referenceScopes)) {
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
