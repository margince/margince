// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The vocabulary comes from report specs and is narrowed before a model sees it.

import (
	"context"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/dealcoverage"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// AnalyticsSchemaFor derives what this caller may ask about from the prebuilt
// catalog alone. AnalyticsSchemaIn adds the installation's reportable custom
// fields, and every door that runs a question uses that one.
func AnalyticsSchemaFor(ctx context.Context) analyticsquery.Schema {
	return analyticsSchemaFrom(ctx, prebuiltReports)
}

// AnalyticsSchemaIn derives what this caller may ask about in this
// installation, and returns the specs it was derived from. The scope a question
// runs under then reads the same fields the vocabulary offered.
func AnalyticsSchemaIn(ctx context.Context, tx pgx.Tx) (analyticsquery.Schema, map[string]reportSpec, error) {
	specs, err := analyticsCatalog(ctx, tx)
	if err != nil {
		return analyticsquery.Schema{}, nil, err
	}
	return analyticsSchemaFrom(ctx, specs), specs, nil
}

func analyticsSchemaFrom(ctx context.Context, specs map[string]reportSpec) analyticsquery.Schema {
	entities := map[string]analyticsquery.Entity{}
	// Record one admission check; the per-population checks below only filter the vocabulary.
	if auth.RequireOneOf(ctx, analyticsPopulationChecks(specs)...) != nil {
		return analyticsquery.Schema{Entities: entities, Version: schemaVersion(entities)}
	}
	for key, spec := range specs {
		// Refuse to name a population whose base entity the caller cannot read.
		if _, err := auth.Unrecorded(ctx, string(spec.entity), principal.ActionRead); err != nil {
			continue
		}
		granted := grantedSpec(ctx, spec)
		entity := analyticsquery.Entity{
			Name: key,
			// Reuse the report engine's aliases and fixed joins.
			From:        granted.fromClause(),
			BaseWhere:   granted.populationWhere(),
			Rows:        spec.basePlain,
			Fields:      map[string]analyticsquery.Field{},
			Contributor: contributorColumn(granted),
		}
		addFields(entity.Fields, analyticsquery.WithoutKeys(granted.dimensions, slices.Concat(granted.rowKeys, granted.rowMoments)), analyticsquery.KindDimension)
		addFields(entity.Fields, granted.measures, analyticsquery.KindMeasure)
		// Narrowed by grant, not spec.filters. A filter reading another record
		// type owes that type's read grant, and naming one the caller lacks
		// would advertise a narrowing that answers 403.
		addFields(entity.Fields, grantedFilters(ctx, spec), analyticsquery.KindFilter)
		addFields(entity.Fields, analyticsquery.OnlyKeys(granted.dimensions, granted.rowMoments), analyticsquery.KindFilter)
		for name, field := range entity.Fields {
			// The same template the report engine renders, so one declaration
			// decides the question both doors ask.
			field.Predicate = spec.filterPredicates[name]
			field.Domain = filterDomain(spec, field.Expr)
			field.Currency = granted.perRowCurrency(name)
			field.Unpriced = granted.sumCoverage[name]
			field.PartialSum = granted.partialSums && field.Unpriced != ""
			field.Period = periodFieldOf(name, field.Expr)
			if custom, ok := spec.customFields[field.Expr]; ok {
				field = custom.describe(field)
			}
			entity.Fields[name] = field
		}
		entity.Coverage = partialSumCoverage(granted, entity.Fields)
		// An entity with nothing left to name is not offered. A population a
		// caller can see the existence of but ask nothing about is a name that
		// only tells them a report exists.
		if len(entity.Fields) == 0 {
			continue
		}
		entities[key] = entity
	}
	return analyticsquery.Schema{Entities: entities, Version: schemaVersion(entities)}
}

// partialSumCoverage is how a population's partial sums state what they leave
// out, or nil when none is offered. A reader not granted the currency gets none listed.
// Rows with a masked amount or currency never reach the counts, because the mask narrows the statement.
func partialSumCoverage(granted reportSpec, fields map[string]analyticsquery.Field) *analyticsquery.MoneyCoverage {
	if !granted.partialSums || !slices.ContainsFunc(slices.Collect(maps.Values(fields)), func(f analyticsquery.Field) bool { return f.PartialSum }) {
		return nil
	}
	currency, ok := granted.dimensions[fieldCurrency]
	if !ok {
		currency = "NULL::text"
	}
	return &analyticsquery.MoneyCoverage{Value: pipelineBaseValueExpr, Amount: dealcoverage.AmountSQL, Currency: currency}
}

func addFields(into map[string]analyticsquery.Field, from map[string]string, kind analyticsquery.FieldKind) {
	for name, expr := range from {
		// A dimension remains groupable when it is also offered as a filter.
		if _, served := into[name]; served {
			continue
		}
		// Aggregate inputs bind as numbers; dimensions use their declared shape.
		shape := columnValueShapes[expr]
		if kind == analyticsquery.KindMeasure {
			shape = analyticsquery.ShapeNumber
		}
		into[name] = analyticsquery.Field{Name: name, Expr: expr, Kind: kind, Shape: shape}
	}
}

// Shapes belong to expressions so two reports cannot type the same field differently.
var columnValueShapes = map[string]analyticsquery.ColumnShape{
	colWinProbability:    analyticsquery.ShapeNumber,
	colBecameOpportunity: analyticsquery.ShapeBoolean,
	// Predicates rather than columns, and boolean for the same reason: the
	// expression yields true or false, so `stalled = true` is the question as
	// anyone would write it.
	deals.StalledSQL("t"):        analyticsquery.ShapeBoolean,
	deals.PartnerSourcedSQL("t"): analyticsquery.ShapeBoolean,
	// Record ids, held by TestEveryIDNamedFieldReadsAnIDShapedColumn.
	colRowID: analyticsquery.ShapeID, colOwnerID: analyticsquery.ShapeID, colPipelineID: analyticsquery.ShapeID,
	colStageID: analyticsquery.ShapeID, colCompanyID: analyticsquery.ShapeID, colPartnerCompanyID: analyticsquery.ShapeID,
	colProjectID: analyticsquery.ShapeID, colProjectCustomer: analyticsquery.ShapeID, colCreditedSeat: analyticsquery.ShapeID,
	colCreditedSeats:       analyticsquery.ShapeID,
	colMeetingCreditedSeat: analyticsquery.ShapeID, activityProjectIDExpr: analyticsquery.ShapeID,
}

// valueShape is the shape of the column a filter or dimension name reads.
func (s reportSpec) valueShape(name string) analyticsquery.ColumnShape {
	expr, _ := predicateExpr(s, name)
	return columnValueShapes[expr]
}

func schemaVersion(entities map[string]analyticsquery.Entity) string {
	return analyticsquery.SchemaVersion(entities)
}

// DescribeAnalyticsSchema renders the vocabulary as prose for a model.
func DescribeAnalyticsSchema(schema analyticsquery.Schema) string {
	return analyticsquery.DescribeSchema(schema)
}

// analyticsPopulationChecks is the read check of each population specs name,
// once each, in a fixed order so the recorded check reads the same every call.
func analyticsPopulationChecks(specs map[string]reportSpec) []auth.Check {
	var objects []string
	for _, spec := range specs {
		objects = append(objects, string(spec.entity))
	}
	slices.Sort(objects)
	checks := make([]auth.Check, 0, len(objects))
	for _, object := range slices.Compact(objects) {
		checks = append(checks, auth.Check{Object: object, Actions: []principal.Action{principal.ActionRead}})
	}
	return checks
}

func requireAnalyticsRead(ctx context.Context, entity datasource.EntityType) error {
	return auth.Require(ctx, string(entity), principal.ActionRead)
}
