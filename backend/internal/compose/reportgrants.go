// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Vocabulary that reads ANOTHER record type than the report's own.
//
// The engine's admission gate covers spec.entity. A name that reads a second
// record type owes that type's read grant too. Asked for without it, the name
// is refused (403); never asked for, it is not served.

import (
	"context"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requireVocabularyGrants refuses the first requested name whose grant the
// caller lacks. A name owes what spec.grants declares and the read grant of any
// table its column references; row scope never stands in for that grant.
func requireVocabularyGrants(ctx context.Context, spec reportSpec, names []string) error {
	for _, name := range names {
		for _, object := range vocabularyObjects(spec, name) {
			if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
				return err
			}
		}
	}
	return nil
}

// vocabularyGranted answers whether the caller holds every grant name owes. It
// asks unrecorded: a missing grant only narrows the vocabulary, so it must not
// hide the tool from a role that lacks it.
func vocabularyGranted(ctx context.Context, spec reportSpec, name string) bool {
	for _, object := range vocabularyObjects(spec, name) {
		if _, err := auth.Unrecorded(ctx, object, principal.ActionRead); err != nil {
			return false
		}
	}
	return true
}

// vocabularyObjects is the record types whose read grant name owes.
func vocabularyObjects(spec reportSpec, name string) []string {
	return slices.DeleteFunc(append(referencedTables(spec, name), spec.grants[name]),
		func(object string) bool { return object == "" })
}

// referencedTables is the table a dimension's, filter's or drill column's
// column references.
func referencedTables(spec reportSpec, name string) (out []string) {
	for _, expr := range []string{spec.dimensions[name], spec.filters[name], spec.drillColumns[name]} {
		if table, scoped := spec.referenceScopes[expr]; scoped && expr != "" {
			out = append(out, table)
		}
	}
	if table, gated := spec.filterScopes[name]; gated {
		out = append(out, table)
	}
	return out
}

// grantedNames filters a vocabulary down to the names this caller may be
// served, for the plans the caller did not spell out themselves.
func grantedNames(ctx context.Context, spec reportSpec, vocabulary map[string]string) []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(vocabulary)) {
		if vocabularyGranted(ctx, spec, name) {
			out = append(out, name)
		}
	}
	return out
}

// grantedFilters is the spec's filter vocabulary minus the names this caller
// may not read, as the expression map the analytics schema publishes.
//
// It returns the map rather than the names grantedNames gives, because the
// schema serves each field's SQL beside its name.
//
// Narrowed here and not in grantedSpec, which feeds the row fetch. A filter
// dropped there would widen the result set rather than refuse it.
func grantedFilters(ctx context.Context, spec reportSpec) map[string]string {
	out := map[string]string{}
	for _, name := range grantedNames(ctx, spec, spec.filters) {
		out[name] = spec.filters[name]
	}
	return out
}

// grantedDefaultAggregates is the spec's default plan minus the measures the
// caller may not read. A count over the report's own rows survives, so the
// narrower seat still gets an answer rather than a refusal for a plan it never
// wrote.
func grantedDefaultAggregates(ctx context.Context, spec reportSpec) []reportAggregate {
	out := make([]reportAggregate, 0, len(spec.defaultAggs))
	for _, agg := range spec.defaultAggs {
		if agg.Field != "" && !vocabularyGranted(ctx, spec, agg.Field) {
			continue
		}
		out = append(out, agg)
	}
	return out
}

// aggregateFields lists the measure names a plan's aggregates read.
func aggregateFields(aggregates []reportAggregate) []string {
	fields := make([]string, 0, len(aggregates))
	for _, agg := range aggregates {
		if agg.Field != "" {
			fields = append(fields, agg.Field)
		}
	}
	return fields
}
