// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// What a link field holds on one record, read for an explanation. The reading
// is built from the field's own Link template, so the rows it lists are the
// rows the field's clause tests.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// linkedValueCap bounds how many linked values one record reports per field.
const linkedValueCap = 50

const linkExistsPrefix = "EXISTS (SELECT 1 FROM "

// LinkedValue is one value behind a link field. Hidden says it names a row the
// reader may not open; its Value is then empty.
type LinkedValue struct {
	Value  string  `json:"value"`
	Label  *string `json:"label,omitempty"`
	Hidden bool    `json:"hidden,omitempty"`
}

// LinkValuesSQL is a scalar subquery giving the text[] of distinct values the
// record carries behind field.Link, at most linkedValueCap of them.
func LinkValuesSQL(field Field, arg func(any) int) (string, error) {
	inner, err := scopedLinkInner(field, "TRUE", arg)
	if err != nil {
		return "", err
	}
	exists := fmt.Sprintf(field.Link, inner)
	if !strings.HasPrefix(exists, linkExistsPrefix) || !strings.HasSuffix(exists, ")") {
		return "", fmt.Errorf("storekit: link %q cannot be read back: it does not start with %q", field.Link, linkExistsPrefix)
	}
	from := strings.TrimSuffix(strings.TrimPrefix(exists, linkExistsPrefix), ")")
	return fmt.Sprintf(
		"(SELECT coalesce(array_agg(v), '{}') FROM (SELECT DISTINCT (%s)::text AS v FROM %s AND (%s) IS NOT NULL ORDER BY v LIMIT %d) linked_values)",
		field.Expr, from, field.Expr, linkedValueCap), nil
}

// fieldReading is how one leaf's field is shown to this reader.
type fieldReading struct {
	hidden bool
	// expr is the SQL of the shown value: a scalar column, or for a linked
	// field the text[] subquery.
	expr   string
	linked bool
}

// readField decides, for explanations and member values alike, whether a leaf's
// field is hidden, a scalar value or a list of linked values.
func (q Query) readField(ctx context.Context, name string, field Field, arg func(any) int) (fieldReading, error) {
	if field.Withheld || (field.Link != "" && !field.ReadsLink) {
		return fieldReading{hidden: true}, nil
	}
	expr, masked, err := q.shownValue(ctx, name, field)
	if err != nil || masked {
		return fieldReading{hidden: masked}, err
	}
	if field.Link == "" {
		return fieldReading{expr: expr}, nil
	}
	linked, err := LinkValuesSQL(field, arg)
	return fieldReading{expr: linked, linked: true}, err
}

// linkLabelTables name the reference targets whose rows carry no row scope, so
// the reader's grant on the vocabulary decides.
var linkLabelTables = map[Reference]string{RefTag: "tag", RefTeam: "team"}

// linkedLookup answers, for each raw value behind a link field, what this
// reader may see of it.
// A row they may not open is Hidden, and one they may open is named.
// Several records share one lookup, so a page reads each table once.
func linkedLookup(ctx context.Context, tx pgx.Tx, ref Reference, values []string) (map[string]LinkedValue, error) {
	found := make(map[string]LinkedValue, len(values))
	if len(values) == 0 {
		return found, nil
	}
	var seen map[string]*string
	var err error
	switch table, scoped := referencedTables[ref]; {
	case ref == "":
	case scoped:
		seen, err = seenReferences(ctx, tx, table, values)
	case linkLabelTables[ref] != "":
		seen, err = namedReferences(ctx, tx, linkLabelTables[ref], values)
	default:
		err = fmt.Errorf("storekit: a %q link has no reading", ref)
	}
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		if ref == "" {
			label := v
			found[v] = LinkedValue{Value: v, Label: &label}
			continue
		}
		if label, ok := seen[v]; ok {
			found[v] = LinkedValue{Value: v, Label: label}
		} else {
			found[v] = LinkedValue{Hidden: true}
		}
	}
	return found, nil
}

// resolveLinked is one record's raw link values as this reader may see them,
// named values first in label order and hidden ones last.
func resolveLinked(found map[string]LinkedValue, values []string) []LinkedValue {
	out := make([]LinkedValue, 0, len(values))
	for _, v := range values {
		out = append(out, found[v])
	}
	sort.SliceStable(out, func(i, j int) bool { return linkedSortKey(out[i]) < linkedSortKey(out[j]) })
	return out
}

func linkedSortKey(v LinkedValue) string {
	switch {
	case v.Hidden:
		return "\xff"
	case v.Label != nil:
		return strings.ToLower(*v.Label)
	}
	return strings.ToLower(v.Value)
}

// namedReferences names rows of an unscoped vocabulary table.
func namedReferences(ctx context.Context, tx pgx.Tx, table string, values []string) (map[string]*string, error) {
	rowIDs, parsed, err := parseReferenceIDs(table, values)
	if err != nil {
		return nil, err
	}
	names, err := LabelsByID(ctx, tx, fmt.Sprintf("SELECT id, coalesce(name, '') FROM %s WHERE id = ANY($1)",
		pgx.Identifier{table}.Sanitize()), rowIDs)
	if err != nil {
		return nil, err
	}
	seen := map[string]*string{}
	for value, id := range parsed {
		if name, ok := names[id]; ok {
			seen[value] = &name
		}
	}
	return seen, nil
}
