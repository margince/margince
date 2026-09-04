// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The ad-hoc datasource plan: the one report surface that BUILDS a spec rather
// than reading one from the catalog.
//
// It runs the same engine the prebuilt reports run — a tool never re-derives
// what the web surface computes — and differs in exactly one declared way: the
// population it measures. See populationRule.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// runAdHocPlan serves the datasource seam's RunReport: the plan's
// vocabulary is the schema descriptors (every declared field may group
// or filter; count is the aggregate). Used by overlay tooling and the
// seam conformance tests rather than the HTTP surface.
func (e *reportEngine) runAdHocPlan(ctx context.Context, plan datasource.ReportPlan) (datasource.ReportResult, error) {
	fields, ok := schemaFields(plan.Entity)
	if !ok {
		return datasource.ReportResult{}, errUnknownEntity
	}
	spec := reportSpec{
		entity:       plan.Entity,
		table:        string(plan.Entity),
		baseWhere:    whereArchivedNull,
		activityWalk: plan.Entity == datasource.EntityActivity,
		dimensions:   map[string]string{},
		measures:     map[string]string{},
		filters:      map[string]string{},
		defaultAggs:  []reportAggregate{{Fn: aggFnCount, As: aliasCount}},
		population:   measureEveryReadableRow,
	}
	for _, f := range fields {
		expr := "t." + f.Name
		spec.dimensions[f.Name] = expr
		spec.filters[f.Name] = expr
		if f.Type == "bigint" || f.Type == "integer" {
			spec.measures[f.Name] = expr
		}
	}
	req := reportRequest{GroupBy: plan.GroupBy, Filters: map[string]any{}}
	for k, v := range plan.Filter {
		req.Filters[k] = v
	}
	outcome, err := e.runSpec(ctx, "adhoc:"+string(plan.Entity), spec, req)
	if err != nil {
		return datasource.ReportResult{}, err
	}
	result := datasource.ReportResult{Columns: outcome.Columns}
	for _, row := range outcome.Rows {
		values := make([]any, len(outcome.Columns))
		for i, col := range outcome.Columns {
			values[i] = row[col]
		}
		result.Rows = append(result.Rows, values)
	}
	return result, nil
}
