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
	// soleKey: a whole-table unique index has this column as its only key.
	soleKey bool
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
	pinned, err := readPinningReferences(ctx, q, namespace+"_")
	if err != nil {
		return fmt.Errorf("extprivacy: reading the references into the %s tables: %w", namespace, err)
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
		if constraint, ok := pinned[t.Name]; ok && Swept(t) {
			return fmt.Errorf("extprivacy: %s is swept by age, but foreign key %s refuses the delete of a row it names; "+
				"declare it ON DELETE CASCADE or ON DELETE SET NULL", t.Name, constraint)
		}
	}
	return nil
}

// readPinningReferences names, per ext table under the prefix, a foreign key
// that would refuse the delete of one of its rows.
func readPinningReferences(ctx context.Context, q Querier, prefix string) (map[string]string, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT ON (target.relname) target.relname, k.conname
		  FROM pg_constraint k
		  JOIN pg_class target ON target.oid = k.confrelid
		  JOIN pg_namespace n ON n.oid = target.relnamespace
		 WHERE k.contype = 'f' AND k.confdeltype NOT IN ('c', 'n')
		   AND n.nspname = $1 AND left(target.relname, length($2::text)) = $2::text
		 ORDER BY target.relname, k.conname`, Schema, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pinned := map[string]string{}
	for rows.Next() {
		var table, constraint string
		if err := rows.Scan(&table, &constraint); err != nil {
			return nil, err
		}
		pinned[table] = constraint
	}
	return pinned, rows.Err()
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
		                                     AND d.refobjid = i.indrelid AND d.refobjsubid = a.attnum))),
		       EXISTS (SELECT 1 FROM pg_index i
		                WHERE i.indrelid = a.attrelid AND i.indisunique AND i.indnkeyatts = 1
		                  AND i.indkey[0] = a.attnum AND i.indpred IS NULL AND i.indexprs IS NULL)
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
		if err := rows.Scan(&table, &name, &c.dataType, &c.nullable, &c.constrained, &c.soleKey); err != nil {
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
	// Only a table the core clears or sweeps needs a row id: it audits each row by it.
	if len(t.Personal) > 0 || Swept(t) {
		id, ok := have["id"]
		if !ok {
			return fmt.Errorf("extprivacy: privacy declares column %s.id, which the table does not have", t.Name)
		}
		if id.dataType != "uuid" || id.nullable || !id.soleKey {
			return fmt.Errorf("extprivacy: %s.id must be a uuid primary key, or a NOT NULL uuid under a unique index of its own, "+
				"because the core audits each cleared row by it", t.Name)
		}
	}
	if err := checkAgeColumn(t, have); err != nil {
		return err
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

// checkAgeColumn holds a declared age column to a point in time every row has.
// A row without one would never be due. A timestamp without a zone is refused:
// the window is an instant, and a wall time is one only in some zone.
func checkAgeColumn(t privacy.Table, have map[string]column) error {
	if t.AgeColumn == "" {
		return nil
	}
	c, ok := have[t.AgeColumn]
	if !ok {
		return fmt.Errorf("extprivacy: privacy declares age column %s.%s, which the table does not have", t.Name, t.AgeColumn)
	}
	if c.dataType != "timestamp with time zone" && c.dataType != "date" {
		return fmt.Errorf("extprivacy: age column %s.%s is %s; a retention sweep needs a timestamptz or a date", t.Name, t.AgeColumn, c.dataType)
	}
	if c.nullable {
		return fmt.Errorf("extprivacy: age column %s.%s is nullable; a row with no age would never be due, so make it NOT NULL", t.Name, t.AgeColumn)
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
