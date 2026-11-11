// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extprivacy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// Querier is the one database call the schema check needs.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// column is what the schema check needs to know of one migrated column.
type column struct {
	dataType    string
	nullable    bool
	constrained bool
}

// VerifySchema compares a unit's declaration with the tables its migrations
// created in the ext schema. Both directions fail. A table the declaration
// leaves out holds personal data nothing would clear. A declared column the
// table lacks would fail the clear when an erasure is due. others are the
// namespaces of the other composed units: a longer one that starts with this
// unit's prefix owns the tables under it.
func VerifySchema(ctx context.Context, q Querier, namespace string, others []string, declared []privacy.Table) error {
	tables, err := readTables(ctx, q, namespace+"_")
	if err != nil {
		return fmt.Errorf("extprivacy: reading the %s tables: %w", namespace, err)
	}
	for table := range tables {
		if ownedByLonger(table, namespace, others) {
			delete(tables, table)
		}
	}
	byName := make(map[string]bool, len(declared))
	for _, t := range declared {
		byName[t.Name] = true
	}
	for table := range tables {
		if !byName[table] {
			return fmt.Errorf("extprivacy: table %s.%s has no privacy declaration; declare its links, personal columns and retention class", Schema, table)
		}
	}
	for _, t := range declared {
		have, ok := tables[t.Name]
		if !ok {
			return fmt.Errorf("extprivacy: privacy declares table %s, which the unit's migrations have not created", t.Name)
		}
		if err := checkColumns(t, have); err != nil {
			return err
		}
	}
	return nil
}

func ownedByLonger(table, namespace string, others []string) bool {
	for _, other := range others {
		if len(other) > len(namespace) && strings.HasPrefix(table, other+"_") {
			return true
		}
	}
	return false
}

// readTables reads every column of the ext tables under the prefix. It marks a
// column a CHECK, foreign key or exclusion constraint names. It also marks one a
// unique index reads, as a key or inside an expression.
func readTables(ctx context.Context, q Querier, prefix string) (map[string]map[string]column, error) {
	rows, err := q.Query(ctx, `
		SELECT c.table_name, c.column_name, c.data_type, c.is_nullable = 'YES',
		       EXISTS (SELECT 1 FROM pg_constraint k
		                WHERE k.conrelid = format('%I.%I', c.table_schema, c.table_name)::regclass
		                  AND k.contype IN ('c', 'f', 'x') AND a.attnum = ANY(k.conkey))
		    OR EXISTS (SELECT 1 FROM pg_index i
		                WHERE i.indrelid = format('%I.%I', c.table_schema, c.table_name)::regclass
		                  AND i.indisunique
		                  AND (a.attnum = ANY(i.indkey)
		                       OR EXISTS (SELECT 1 FROM pg_depend d
		                                   WHERE d.classid = 'pg_class'::regclass AND d.objid = i.indexrelid
		                                     AND d.refobjid = i.indrelid AND d.refobjsubid = a.attnum)))
		  FROM information_schema.columns c
		  JOIN pg_attribute a ON a.attrelid = format('%I.%I', c.table_schema, c.table_name)::regclass
		                     AND a.attname = c.column_name
		 WHERE c.table_schema = $1 AND left(c.table_name, length($2::text)) = $2::text`, Schema, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := map[string]map[string]column{}
	for rows.Next() {
		var table, name string
		var c column
		if err := rows.Scan(&table, &name, &c.dataType, &c.nullable, &c.constrained); err != nil {
			return nil, err
		}
		if tables[table] == nil {
			tables[table] = map[string]column{}
		}
		tables[table][name] = c
	}
	return tables, rows.Err()
}

// checkColumns holds one declared table to the columns the migrated table has,
// and to the clear the core can run on each personal one.
func checkColumns(t privacy.Table, have map[string]column) error {
	// Only a table the core clears needs a row id: it audits each cleared row by it.
	if len(t.Personal) > 0 {
		id, ok := have["id"]
		if !ok {
			return fmt.Errorf("extprivacy: privacy declares column %s.id, which the table does not have", t.Name)
		}
		if id.dataType != "uuid" {
			return fmt.Errorf("extprivacy: %s.id must be a uuid, because the core audits each cleared row by it", t.Name)
		}
	}
	for _, name := range t.Personal {
		c, ok := have[name]
		if !ok {
			return fmt.Errorf("extprivacy: privacy declares column %s.%s, which the table does not have", t.Name, name)
		}
		if err := clearable(c); err != nil {
			return fmt.Errorf("extprivacy: personal column %s.%s: %w", t.Name, name, err)
		}
	}
	for _, l := range t.Links {
		c, ok := have[l.Column]
		if !ok {
			return fmt.Errorf("extprivacy: privacy declares column %s.%s, which the table does not have", t.Name, l.Column)
		}
		if c.dataType != "uuid" {
			return fmt.Errorf("extprivacy: link column %s.%s is %s; it must be a uuid, the type of a core record id", t.Name, l.Column, c.dataType)
		}
	}
	return nil
}

// clearable reports whether the core can blank a personal column. A required
// one takes its type's empty value, which a constraint on it may refuse.
func clearable(c column) error {
	if _, err := emptyValue(c.dataType, c.nullable); err != nil {
		return err
	}
	if !c.nullable && c.constrained {
		return errors.New("a required personal column under a CHECK, UNIQUE, foreign key or exclusion constraint may refuse the blank value; make it nullable")
	}
	return nil
}
