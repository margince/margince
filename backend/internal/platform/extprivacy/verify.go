// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extprivacy

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// Querier is the one database call the schema check needs.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// VerifySchema compares a unit's declaration with the tables its migrations
// created in the ext schema. Both directions fail. A table the declaration
// leaves out holds personal data nothing would clear. A declared column the
// table lacks would fail the clear when an erasure is due.
func VerifySchema(ctx context.Context, q Querier, namespace string, declared []privacy.Table) error {
	prefix := namespace + "_"
	rows, err := q.Query(ctx, `
		SELECT table_name, column_name, data_type, is_nullable = 'YES' FROM information_schema.columns
		 WHERE table_schema = $1 AND left(table_name, length($2::text)) = $2::text`, Schema, prefix)
	if err != nil {
		return fmt.Errorf("extprivacy: reading the %s tables: %w", namespace, err)
	}
	defer rows.Close()
	columns := map[string][]string{}
	unclearable := map[string]error{}
	dataTypes := map[string]string{}
	for rows.Next() {
		var table, column, dataType string
		var nullable bool
		if err := rows.Scan(&table, &column, &dataType, &nullable); err != nil {
			return err
		}
		columns[table] = append(columns[table], column)
		dataTypes[table+"."+column] = dataType
		if column == "id" {
			if dataType != "uuid" {
				unclearable[table+".id"] = errors.New("the row id must be a uuid, because the core audits each cleared row by it")
			}
			continue
		}
		if _, err := emptyValue(dataType, nullable); err != nil {
			unclearable[table+"."+column] = err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	byName := make(map[string]privacy.Table, len(declared))
	for _, t := range declared {
		byName[t.Name] = t
	}
	for table := range columns {
		if _, ok := byName[table]; !ok {
			return fmt.Errorf("extprivacy: table %s.%s has no privacy declaration; declare its links, personal columns and retention class", Schema, table)
		}
	}
	for _, t := range declared {
		have, ok := columns[t.Name]
		if !ok {
			return fmt.Errorf("extprivacy: privacy declares table %s, which the unit's migrations have not created", t.Name)
		}
		if err := checkColumns(t, have, unclearable); err != nil {
			return err
		}
		if err := checkAgeColumn(t, dataTypes); err != nil {
			return err
		}
	}
	return nil
}

// checkAgeColumn holds a declared age column to being a point in time, the only
// thing a retention window can be counted from.
func checkAgeColumn(t privacy.Table, dataTypes map[string]string) error {
	if t.AgeColumn == "" {
		return nil
	}
	switch dataTypes[t.Name+"."+t.AgeColumn] {
	case "timestamp with time zone", "timestamp without time zone", "date":
		return nil
	case "":
		return fmt.Errorf("extprivacy: privacy declares age column %s.%s, which the table does not have", t.Name, t.AgeColumn)
	}
	return fmt.Errorf("extprivacy: age column %s.%s is a %s; a retention sweep needs a timestamp or a date",
		t.Name, t.AgeColumn, dataTypes[t.Name+"."+t.AgeColumn])
}

// checkColumns holds one declared table to the columns the migrated table has,
// and to the clear the core can run on each personal one.
func checkColumns(t privacy.Table, have []string, unclearable map[string]error) error {
	// Only a table the core clears, holds or sweeps needs a row id: it audits each
	// row by it.
	var need []string
	if identifiable(t) {
		need = append(need, "id")
	}
	for _, c := range append(slices.Clone(need), t.Personal...) {
		if err := unclearable[t.Name+"."+c]; err != nil {
			return fmt.Errorf("extprivacy: personal column %s.%s: %w", t.Name, c, err)
		}
	}
	need = append(need, t.Personal...)
	for _, l := range t.Links {
		need = append(need, l.Column)
	}
	for _, c := range need {
		if !slices.Contains(have, c) {
			return fmt.Errorf("extprivacy: privacy declares column %s.%s, which the table does not have", t.Name, c)
		}
	}
	return nil
}
