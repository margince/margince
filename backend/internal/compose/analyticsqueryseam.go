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

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
)

// AnalyticsSchemaFor derives what this caller may ask about.
func AnalyticsSchemaFor(ctx context.Context) analyticsquery.Schema {
	entities := map[string]analyticsquery.Entity{}
	for key, spec := range prebuiltReports {
		granted := grantedSpec(ctx, spec)
		entity := analyticsquery.Entity{
			Name: key,
			// fromClause() and not the bare table: it carries the alias `t`
			// every spec expression is written against, plus the spec's fixed
			// lookup joins. Reassembling it here would be a second spelling of
			// the report engine's FROM, and the two would disagree the first
			// time a spec grew a join.
			From:      granted.fromClause(),
			BaseWhere: granted.baseWhere,
			Fields:    map[string]analyticsquery.Field{},
		}
		addFields(entity.Fields, granted.dimensions, analyticsquery.KindDimension)
		addFields(entity.Fields, granted.measures, analyticsquery.KindMeasure)
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

func addFields(into map[string]analyticsquery.Field, from map[string]string, kind analyticsquery.FieldKind) {
	for name, expr := range from {
		into[name] = analyticsquery.Field{Name: name, Expr: expr, Kind: kind}
	}
}

// schemaVersion is a digest of the vocabulary this caller was handed.
//
// It rides on a compiled plan so a plan can be refused after the schema moves:
// a plan naming a field that has since been renamed would otherwise render SQL
// against a column that no longer exists, and the caller would read a database
// error where they should read "ask again".
//
// Per-caller rather than installation-wide, deliberately. A seat that LOSES a
// grant must have its outstanding plans refused too, and an installation-wide
// version would not move when one person's roles changed.
func schemaVersion(entities map[string]analyticsquery.Entity) string {
	sum := sha256.New()
	for _, name := range slices.Sorted(maps.Keys(entities)) {
		sum.Write([]byte(name))
		entity := entities[name]
		for _, field := range slices.Sorted(maps.Keys(entity.Fields)) {
			sum.Write([]byte(field))
			sum.Write([]byte(entity.Fields[field].Expr))
			sum.Write([]byte(entity.Fields[field].Kind))
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
	}
	return out.String()
}
