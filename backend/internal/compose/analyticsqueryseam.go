// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The vocabulary a generic question may be asked in, derived from the report
// catalog and narrowed by the caller's grants.
//
// DERIVED, not declared. Every entity, dimension and measure here is one a
// prebuilt report already groups or aggregates by, which buys two things a
// hand-written catalog would not: the row-scope and reference-scope machinery
// already knows how to gate each field, and a column added to a spec appears
// here without anybody remembering to add it twice.
//
// The narrowing happens BEFORE the query is planned. A field the caller may not
// read is absent from their schema, so naming it answers "no such field" — the
// alternative sentence, "you may not read that", tells somebody the column
// exists, which is the disclosure the grant was for.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
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
	for key, spec := range specs {
		// The population's OWN read gate, applied at derivation. grantedSpec
		// below narrows only the fields that read ANOTHER record type
		// (spec.grants); the base entity's grant is the engine's admission
		// gate, and a derivation that skipped it would name populations —
		// here, in the vocabulary document, and in the unknown-name refusal —
		// to a seat that may not read a row of them.
		if requireAnalyticsRead(ctx, spec.entity) != nil {
			continue
		}
		granted := grantedSpec(ctx, spec)
		entity := analyticsquery.Entity{
			Name: key,
			// fromClause() and not the bare table: it carries the alias `t`
			// every spec expression is written against, plus the spec's fixed
			// lookup joins. Reassembling it here would be a second spelling of
			// the report engine's FROM, and the two would disagree the first
			// time a spec grew a join.
			From:      granted.fromClause(),
			BaseWhere: granted.populationWhere(),
			Fields:    map[string]analyticsquery.Field{},
		}
		addFields(entity.Fields, withoutKeys(granted.dimensions, slices.Concat(granted.rowKeys, granted.rowMoments)), analyticsquery.KindDimension)
		addFields(entity.Fields, granted.measures, analyticsquery.KindMeasure)
		// Narrowed by grant, not spec.filters. A filter reading another record
		// type owes that type's read grant, and naming one the caller lacks
		// would advertise a narrowing that answers 403.
		addFields(entity.Fields, grantedFilters(ctx, spec), analyticsquery.KindFilter)
		addFields(entity.Fields, onlyKeys(granted.dimensions, granted.rowMoments), analyticsquery.KindFilter)
		for name, field := range entity.Fields {
			field.Domain = filterDomain(spec, field.Expr)
			field.Currency = granted.perRowCurrency(name)
			if custom, ok := spec.customFields[field.Expr]; ok {
				field = custom.describe(field)
			}
			entity.Fields[name] = field
		}
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

func withoutKeys(from map[string]string, keys []string) map[string]string {
	out := make(map[string]string, len(from))
	for name, expr := range from {
		if !slices.Contains(keys, name) {
			out[name] = expr
		}
	}
	return out
}

func onlyKeys(from map[string]string, keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, name := range keys {
		if expr, ok := from[name]; ok {
			out[name] = expr
		}
	}
	return out
}

func addFields(into map[string]analyticsquery.Field, from map[string]string, kind analyticsquery.FieldKind) {
	for name, expr := range from {
		// A name already served keeps the kind it has. Most filters are also
		// dimensions, and a dimension is the more capable of the two: it can
		// be grouped on as well as narrowed by. Overwriting it with the filter
		// kind would take the grouping away.
		if _, served := into[name]; served {
			continue
		}
		// A measure is a number by construction — it is the thing an aggregate
		// adds up — so a caller's 1000 binds to it as readily as their "1000".
		// A dimension is text unless its column says otherwise.
		shape := columnValueShapes[expr]
		if kind == analyticsquery.KindMeasure {
			shape = analyticsquery.ShapeNumber
		}
		into[name] = analyticsquery.Field{Name: name, Expr: expr, Kind: kind, Shape: shape}
	}
}

// columnValueShapes are the EXPRESSIONS whose value is something other than
// text. A filter against one carries the JSON literal a question is naturally
// written in, rather than a quoted spelling of it.
//
// Keyed by the expression rather than declared per spec, because what a column
// holds is the COLUMN's property and not each spec's that exposes it:
// win_probability is a dimension of two reports today, and a per-spec
// declaration would let the two disagree about it.
//
// A column missing from here is compared as text, which refuses its literal
// with the quoted spelling that binds — recoverable advice, never the opaque
// fault this list exists to keep a caller out of.
var columnValueShapes = map[string]analyticsquery.ColumnShape{
	colWinProbability:    analyticsquery.ShapeNumber,
	colBecameOpportunity: analyticsquery.ShapeBoolean,
	// Predicates rather than columns, and boolean for the same reason: the
	// expression yields true or false, so `stalled = true` is the question as
	// anyone would write it.
	deals.StalledSQL("t"):        analyticsquery.ShapeBoolean,
	deals.PartnerSourcedSQL("t"): analyticsquery.ShapeBoolean,
}

// schemaVersion is a digest of the vocabulary this caller was handed.
//
// Every answer echoes it, so a caller comparing two answers can tell whether
// they were asked in the same vocabulary. Nothing refuses on it: a question
// carries no version and is compiled against the vocabulary current when it
// runs, so a field that has since gone is refused by name.
//
// Per-caller rather than installation-wide, deliberately. A seat that LOSES a
// grant is asking in a different vocabulary, and an installation-wide version
// would not move when one contact's roles changed.
func schemaVersion(entities map[string]analyticsquery.Entity) string {
	sum := sha256.New()
	for _, name := range slices.Sorted(maps.Keys(entities)) {
		sum.Write([]byte(name))
		entity := entities[name]
		// The population is part of the meaning: a saved answer over a base
		// predicate that has since narrowed must not look comparable to today's.
		sum.Write([]byte(entity.BaseWhere))
		for _, field := range slices.Sorted(maps.Keys(entity.Fields)) {
			sum.Write([]byte(field))
			sum.Write([]byte(entity.Fields[field].Expr))
			sum.Write([]byte(entity.Fields[field].Kind))
			writeCustomMeaning(sum, entity.Fields[field])
		}
	}
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// DescribeAnalyticsSchema renders the vocabulary as prose for a model.
//
// Prose rather than JSON because it reaches a model inside a tool description
// budget, and a nested object spends most of that budget on punctuation.
func DescribeAnalyticsSchema(schema analyticsquery.Schema) string {
	var out strings.Builder
	for _, name := range schema.EntityNames() {
		entity := schema.Entities[name]
		out.WriteString(name)
		out.WriteString("\n  group by: ")
		out.WriteString(strings.Join(entity.FieldNames(analyticsquery.KindDimension), ", "))
		out.WriteString("\n  measure:  ")
		out.WriteString(strings.Join(entity.FieldNames(analyticsquery.KindMeasure), ", "))
		out.WriteString("\n")
		describeCustomFields(&out, entity)
	}
	return out.String()
}

func requireAnalyticsRead(ctx context.Context, entity datasource.EntityType) error {
	return auth.Require(ctx, string(entity), principal.ActionRead)
}
