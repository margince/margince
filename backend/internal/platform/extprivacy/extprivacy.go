// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extprivacy applies the personal-data declarations of extension
// tables. It finds the rows that belong to a core record and clears their
// personal columns. It serves them for a subject export, and counts the rows
// that depend on a record. It writes no audit row and no event. The callers own
// those, so each action keeps its audit shape.
package extprivacy

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// Schema is where every extension table lives.
const Schema = "ext"

// Registry is the composed set of declared tables.
type Registry struct{ tables []privacy.Table }

// New builds a registry from the declarations of the composed units.
func New(tables []privacy.Table) Registry { return Registry{tables: tables} }

// Tables returns the declared tables.
func (r Registry) Tables() []privacy.Table { return r.tables }

// Cleared is one row whose personal columns were cleared.
type Cleared struct {
	Table string
	ID    ids.UUID
}

func qualified(table string) string { return pgx.Identifier{Schema, table}.Sanitize() }

// linked returns the tables' link columns that point at the given kind.
func (r Registry) linked(kind privacy.RecordKind) []linkedColumn {
	var out []linkedColumn
	for _, t := range r.tables {
		for _, l := range t.Links {
			if l.Record == kind {
				out = append(out, linkedColumn{table: t, column: l.Column})
			}
		}
	}
	return out
}

type linkedColumn struct {
	table  privacy.Table
	column string
}

// CountDependents counts the rows of every declared table that name the record.
func (r Registry) CountDependents(ctx context.Context, tx pgx.Tx, kind privacy.RecordKind, id ids.UUID) (int64, error) {
	var total int64
	for _, lc := range r.linked(kind) {
		var n int64
		q := "SELECT count(*) FROM " + qualified(lc.table.Name) + " WHERE " + pgx.Identifier{lc.column}.Sanitize() + " = $1"
		if err := tx.QueryRow(ctx, q, id).Scan(&n); err != nil {
			return 0, fmt.Errorf("extprivacy: counting rows of %s that name the record: %w", lc.table.Name, err)
		}
		total += n
	}
	return total, nil
}

// forEachLink visits every declared link column that points at one of the
// subjects' record kinds, in a fixed order, with the ids of that kind.
func (r Registry) forEachLink(subjects map[privacy.RecordKind][]ids.UUID, visit func(lc linkedColumn, records []ids.UUID) error) error {
	for _, kind := range slices.Sorted(maps.Keys(subjects)) {
		records := subjects[kind]
		if len(records) == 0 {
			continue
		}
		for _, lc := range r.linked(kind) {
			if err := visit(lc, records); err != nil {
				return err
			}
		}
	}
	return nil
}

// Clear overwrites the personal columns of every row that names one of the
// records, and returns the rows it cleared. A nullable column becomes NULL; a
// required one takes the empty value of its type. A row that two links name is
// cleared twice and returned once.
func (r Registry) Clear(ctx context.Context, tx pgx.Tx, subjects map[privacy.RecordKind][]ids.UUID) ([]Cleared, error) {
	var cleared []Cleared
	seen := map[Cleared]bool{}
	// One lookup of a table's column types per call, however many links name it.
	assignments := map[string]string{}
	err := r.forEachLink(subjects, func(lc linkedColumn, records []ids.UUID) error {
		if len(lc.table.Personal) == 0 {
			return nil
		}
		set, ok := assignments[lc.table.Name]
		if !ok {
			var err error
			if set, err = clearAssignments(ctx, tx, lc.table); err != nil {
				return err
			}
			assignments[lc.table.Name] = set
		}
		rows, err := clearLinked(ctx, tx, lc, set, records)
		if err != nil {
			return err
		}
		for _, c := range rows {
			if !seen[c] {
				seen[c] = true
				cleared = append(cleared, c)
			}
		}
		return nil
	})
	return cleared, err
}

// clearLinked runs the clearing UPDATE for one link column and returns the rows
// it touched.
func clearLinked(ctx context.Context, tx pgx.Tx, lc linkedColumn, set string, records []ids.UUID) ([]Cleared, error) {
	rows, err := tx.Query(ctx, "UPDATE "+qualified(lc.table.Name)+" SET "+set+
		" WHERE "+pgx.Identifier{lc.column}.Sanitize()+" = ANY($1) RETURNING id", records)
	if err != nil {
		return nil, fmt.Errorf("extprivacy: clearing %s: %w", lc.table.Name, err)
	}
	defer rows.Close()
	var out []Cleared
	for rows.Next() {
		c := Cleared{Table: lc.table.Name}
		if err := rows.Scan(&c.ID); err != nil {
			return nil, fmt.Errorf("extprivacy: reading a cleared row of %s: %w", lc.table.Name, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("extprivacy: reading the cleared rows of %s: %w", lc.table.Name, err)
	}
	return out, nil
}

// clearAssignments renders the SET list for a table's personal columns from the
// live column types. The unit declares which columns are personal. The core
// decides what "cleared" means for each.
func clearAssignments(ctx context.Context, tx pgx.Tx, t privacy.Table) (string, error) {
	rows, err := tx.Query(ctx, `
		SELECT column_name, data_type, is_nullable = 'YES'
		  FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2 AND column_name = ANY($3)`,
		Schema, t.Name, t.Personal)
	if err != nil {
		return "", fmt.Errorf("extprivacy: reading the columns of %s: %w", t.Name, err)
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var name, dataType string
		var nullable bool
		if err := rows.Scan(&name, &dataType, &nullable); err != nil {
			return "", fmt.Errorf("extprivacy: reading a column of %s: %w", t.Name, err)
		}
		empty, err := emptyValue(dataType, nullable)
		if err != nil {
			return "", fmt.Errorf("extprivacy: %s.%s: %w", t.Name, name, err)
		}
		parts = append(parts, pgx.Identifier{name}.Sanitize()+" = "+empty)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("extprivacy: reading the columns of %s: %w", t.Name, err)
	}
	if len(parts) != len(t.Personal) {
		return "", fmt.Errorf("extprivacy: %s declares %d personal columns and the table has %d of them", t.Name, len(t.Personal), len(parts))
	}
	return strings.Join(parts, ", "), nil
}

func emptyValue(dataType string, nullable bool) (string, error) {
	if nullable {
		return "NULL", nil
	}
	switch dataType {
	case "text", "character varying":
		return "''", nil
	case "bytea":
		return `'\x'::bytea`, nil
	case "jsonb":
		return "'{}'::jsonb", nil
	}
	return "", errors.New("a required personal column of type " + dataType + " has no empty value; make it nullable")
}

// Export returns, per declared table, the id and personal columns of every row
// that names one of the records. A row that two links name is returned once.
func (r Registry) Export(ctx context.Context, tx pgx.Tx, subjects map[privacy.RecordKind][]ids.UUID) ([]map[string]any, error) {
	var out []map[string]any
	seen := map[Cleared]bool{}
	err := r.forEachLink(subjects, func(lc linkedColumn, records []ids.UUID) error {
		found, err := exportLinked(ctx, tx, lc, records)
		if err != nil {
			return err
		}
		for _, f := range found {
			if key := (Cleared{Table: lc.table.Name, ID: f.id}); !seen[key] {
				seen[key] = true
				out = append(out, f.row)
			}
		}
		return nil
	})
	return out, err
}

type exportedRow struct {
	id  ids.UUID
	row map[string]any
}

// exportLinked reads the declared columns of the rows one link column names.
func exportLinked(ctx context.Context, tx pgx.Tx, lc linkedColumn, records []ids.UUID) ([]exportedRow, error) {
	q := `SELECT t.id, (SELECT coalesce(jsonb_object_agg(e.key, e.value), '{}'::jsonb)
	                      FROM jsonb_each(to_jsonb(t)) e WHERE e.key = ANY($2))
	        FROM ` + qualified(lc.table.Name) + ` t WHERE t.` + pgx.Identifier{lc.column}.Sanitize() + ` = ANY($1) ORDER BY t.id`
	rows, err := tx.Query(ctx, q, records, lc.table.Personal)
	if err != nil {
		return nil, fmt.Errorf("extprivacy: exporting %s: %w", lc.table.Name, err)
	}
	defer rows.Close()
	var out []exportedRow
	for rows.Next() {
		var id ids.UUID
		var data map[string]any
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("extprivacy: reading an exported row of %s: %w", lc.table.Name, err)
		}
		out = append(out, exportedRow{id: id, row: map[string]any{
			"table": lc.table.Name, "id": id, "retention_class": string(lc.table.Retention), "personal_data": data,
		}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("extprivacy: reading the rows of %s: %w", lc.table.Name, err)
	}
	return out, nil
}
