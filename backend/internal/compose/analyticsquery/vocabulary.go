// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"maps"
	"slices"
	"strings"
)

// SchemaVersion follows the caller's visible vocabulary so losing a grant
// changes the version even when the installation's schema stays the same.
func SchemaVersion(entities map[string]Entity) string {
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
			sum.Write([]byte(entity.Fields[field].Predicate))
			writeCustomMeaning(sum, entity.Fields[field])
			if entity.Fields[field].PartialSum {
				// A partial sum answers where it once answered null, so a client
				// that predates the coverage must see the vocabulary change.
				sum.Write([]byte("partial-sum:" + CoverageColumnName))
			}
			if money := entity.Fields[field].Currency; money != nil {
				sum.Write([]byte("currency:" + money.By + ":" + money.Base))
			}
		}
	}
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// DescribeSchema renders the vocabulary as prose for a model.
//
// Prose rather than JSON because it reaches a model inside a tool description
// budget, and a nested object spends most of that budget on punctuation.
func DescribeSchema(schema Schema) string {
	var out strings.Builder
	for _, name := range schema.EntityNames() {
		entity := schema.Entities[name]
		out.WriteString(name)
		if entity.Rows != "" {
			out.WriteString("\n  rows:     " + entity.Rows)
		}
		out.WriteString("\n  group by: ")
		out.WriteString(strings.Join(entity.FieldNames(KindDimension), ", "))
		out.WriteString("\n  measure:  ")
		out.WriteString(strings.Join(entity.FieldNames(KindMeasure), ", "))
		out.WriteString("\n")
		describeCurrencyBasis(&out, entity)
		describeCustomFields(&out, entity)
	}
	return out.String()
}

// describeCurrencyBasis names the measures kept in each row's own currency, so a
// model groups them by currency instead of learning the rule from a refusal.
func describeCurrencyBasis(out *strings.Builder, entity Entity) {
	for _, name := range entity.FieldNames(KindMeasure) {
		money := entity.Fields[name].Currency
		if money == nil {
			continue
		}
		fmt.Fprintf(out, "  %s is in each row's own currency: group by %s", name, money.By)
		if money.Base != "" {
			fmt.Fprintf(out, ", or measure %s, converted to the base currency", money.Base)
		}
		out.WriteString("\n")
	}
}

// describeCustomFields names each custom dimension for a model, which has no
// other way to learn what an installation's cf_ column means. An administrator
// typed the labels and choices, so each is JSON-quoted. A comma or a line break
// inside one then cannot read as a second choice or a new line.
func describeCustomFields(out *strings.Builder, entity Entity) {
	header := false
	for _, name := range entity.FieldNames(KindDimension) {
		field := entity.Fields[name]
		if field.Label == "" {
			continue
		}
		if !header {
			out.WriteString("  custom fields, each with the label and choices an administrator gave it:\n")
			header = true
		}
		choices := field.Options
		if field.Shape == ShapeBoolean {
			choices = []string{"true", "false"}
		}
		fmt.Fprintf(out, "    %s %s: %s", name, jsonText(field.Label), jsonText(choices))
		if strings.HasPrefix(name, CompanyCustomFieldPrefix) {
			out.WriteString(", the deal's company's field")
		}
		if field.Coverage != nil {
			fmt.Fprintf(out, ", held by %d of %d live deals you can read", field.Coverage.WithValue, field.Coverage.Records)
		}
		out.WriteString("\n")
	}
}

// writeCustomMeaning adds a custom dimension's label and choices to the schema
// digest, so a renamed field or changed choice moves the version. A built-in
// field writes nothing, which keeps every existing version as it was.
func writeCustomMeaning(sum hash.Hash, field Field) {
	if field.Label == "" {
		return
	}
	sum.Write([]byte(jsonText(field.Label) + jsonText(field.Options)))
}

// jsonText is v as JSON, with < > & left as typed, since a model reads it and
// no browser does. A string or a slice of strings always encodes.
func jsonText[T string | []string](v T) string {
	var out strings.Builder
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// WithoutKeys excludes fields that the caller cannot use in this part of a query.
func WithoutKeys(from map[string]string, keys []string) map[string]string {
	out := make(map[string]string, len(from))
	for name, expr := range from {
		if !slices.Contains(keys, name) {
			out[name] = expr
		}
	}
	return out
}

// OnlyKeys preserves the fields admitted for this part of a query.
func OnlyKeys(from map[string]string, keys []string) map[string]string {
	out := make(map[string]string, len(keys))
	for _, name := range keys {
		if expr, ok := from[name]; ok {
			out[name] = expr
		}
	}
	return out
}
