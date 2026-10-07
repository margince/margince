// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// What a filter's own fields hold on the records it selects, and which of many
// filters select one record. Both read through the compiled filter and the
// caller's row scope, and a value is shown exactly as Explain shows a leaf's.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// FieldValue is one field's value on one record as this reader may see it:
// the value as text, nil when the record holds none, or Hidden.
type FieldValue struct {
	Value  *string
	Hidden bool
	// Label is the name of the record a reference value names, when the
	// reader may open it.
	Label *string
}

// Leaves is the comparison leaves of a filter tree, in the order the tree
// first reaches them.
func Leaves(p Predicate) []Predicate {
	var out []Predicate
	for _, branch := range p.And {
		out = append(out, Leaves(branch)...)
	}
	for _, branch := range p.Or {
		out = append(out, Leaves(branch)...)
	}
	if p.Field != "" {
		out = append(out, p)
	}
	return out
}

// FieldsNamed is the fields a filter tree's leaves name, once each, in the
// order the tree first names them.
func FieldsNamed(p Predicate) []string {
	var out []string
	for _, leaf := range Leaves(p) {
		if !slices.Contains(out, leaf.Field) {
			out = append(out, leaf.Field)
		}
	}
	return out
}

// valueColumn is one shown field of SelectedValues' statement.
type valueColumn struct {
	name       string
	references string
}

// SelectedValues reads, for each of recordIDs the filter selects for this
// caller, the value of every field the filter names. A record the filter does
// not select, or the caller cannot see, is absent from the answer.
func (q Query) SelectedValues(ctx context.Context, tx pgx.Tx, p Predicate, recordIDs []ids.UUID) (map[ids.UUID]map[string]FieldValue, error) {
	where, args, err := q.predicateWhere(ctx, p)
	if err != nil {
		return nil, err
	}
	var shown []valueColumn
	var hidden []string
	selects := []string{"t.id"}
	for _, name := range FieldsNamed(p) {
		field := q.Fields[name]
		masked := field.Withheld || field.Link != ""
		var expr string
		if !masked {
			if expr, masked, err = q.shownValue(ctx, name, field); err != nil {
				return nil, err
			}
		}
		if masked {
			hidden = append(hidden, name)
			continue
		}
		shown = append(shown, valueColumn{name: name, references: referencedTables[field.References]})
		selects = append(selects, "("+expr+")::text")
	}
	args = append(args, recordIDs)
	sql := fmt.Sprintf("SELECT %s FROM %s t WHERE %s AND t.id = ANY($%d)",
		strings.Join(selects, ", "), q.Table, where, len(args))
	out, err := scanValues(ctx, tx, sql, args, shown, hidden)
	if err != nil {
		return nil, fmt.Errorf("filter values on %s: %w", q.Table, err)
	}
	return out, withholdUnseenValues(ctx, tx, out, shown)
}

func scanValues(ctx context.Context, tx pgx.Tx, sql string, args []any, shown []valueColumn, hidden []string) (map[ids.UUID]map[string]FieldValue, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.UUID]map[string]FieldValue{}
	for rows.Next() {
		var id ids.UUID
		texts := make([]*string, len(shown))
		targets := []any{&id}
		for i := range texts {
			targets = append(targets, &texts[i])
		}
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		values := make(map[string]FieldValue, len(shown)+len(hidden))
		for _, name := range hidden {
			values[name] = FieldValue{Hidden: true}
		}
		for i, column := range shown {
			values[column.name] = FieldValue{Value: texts[i]}
		}
		out[id] = values
	}
	return out, rows.Err()
}

// withholdUnseenValues hides each shown value naming a row the reader may not
// open, and names each one they may, as withholdUnseenReferences does for one
// explained record.
func withholdUnseenValues(ctx context.Context, tx pgx.Tx, values map[ids.UUID]map[string]FieldValue, shown []valueColumn) error {
	for _, column := range shown {
		if column.references == "" {
			continue
		}
		var named []string
		for _, record := range values {
			if v := record[column.name].Value; v != nil {
				named = append(named, *v)
			}
		}
		seen, err := seenReferences(ctx, tx, column.references, named)
		if err != nil {
			return err
		}
		for _, record := range values {
			v := record[column.name].Value
			if v == nil {
				continue
			}
			label, ok := seen[*v]
			if !ok {
				record[column.name] = FieldValue{Hidden: true}
				continue
			}
			record[column.name] = FieldValue{Value: v, Label: label}
		}
	}
	return nil
}

// referenceNames is the column naming a row of each referenced table.
var referenceNames = map[string]string{"company": "display_name", "project": "name"}

// seenReferences answers, of values each the id of a row of table, those this
// reader may open, each with the row's name (nil where it has none). A value
// absent from the answer names a row they may not open.
func seenReferences(ctx context.Context, tx pgx.Tx, table string, values []string) (map[string]*string, error) {
	if len(values) == 0 {
		return map[string]*string{}, nil
	}
	parsed := make(map[string]ids.UUID, len(values))
	rowIDs := make([]ids.UUID, 0, len(values))
	for _, value := range values {
		id, err := ids.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("a %s reference is not an id: %w", table, err)
		}
		parsed[value] = id
		rowIDs = append(rowIDs, id)
	}
	visible, err := auth.VisibleSubset(ctx, tx, table, rowIDs)
	if err != nil {
		return nil, err
	}
	var open []ids.UUID
	for _, id := range rowIDs {
		if visible[id] {
			open = append(open, id)
		}
	}
	names, err := LabelsByID(ctx, tx, fmt.Sprintf("SELECT id, coalesce(%s, '') FROM %s WHERE id = ANY($1)",
		pgx.Identifier{referenceNames[table]}.Sanitize(), pgx.Identifier{table}.Sanitize()), open)
	if err != nil {
		return nil, err
	}
	seen := map[string]*string{}
	for value, id := range parsed {
		if !visible[id] {
			continue
		}
		if name, ok := names[id]; ok {
			seen[value] = &name
		} else {
			seen[value] = nil
		}
	}
	return seen, nil
}

// SelectsEach answers whether each filter selects one record, judged as a
// member read judges it: base clause, compiled filter and the caller's row
// scope. A record that clause or scope excludes is selected by none.
func (q Query) SelectsEach(ctx context.Context, tx pgx.Tx, filters []Predicate, recordID ids.UUID) ([]bool, error) {
	out := make([]bool, len(filters))
	if len(filters) == 0 {
		return out, nil
	}
	var columns []string
	where, args, err := q.scopedWhere(ctx, func(arg func(any) int) (string, error) {
		for _, filter := range filters {
			compiled, err := CompilePredicate(filter, q.Fields, arg)
			if err != nil {
				return "", err
			}
			columns = append(columns, "COALESCE(("+compiled+"), false)")
		}
		return fmt.Sprintf("t.id = $%d", arg(recordID)), nil
	})
	if err != nil {
		return nil, err
	}
	targets := make([]any, len(out))
	for i := range out {
		targets[i] = &out[i]
	}
	sql := fmt.Sprintf("SELECT %s FROM %s t WHERE %s", strings.Join(columns, ", "), q.Table, where)
	if err := tx.QueryRow(ctx, sql, args...).Scan(targets...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return make([]bool, len(filters)), nil
		}
		return nil, fmt.Errorf("filters for one record on %s: %w", q.Table, err)
	}
	return out, nil
}
