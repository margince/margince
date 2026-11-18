// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Every column type the schema holds must leave an export as the value it
// holds, never as a Go struct the driver decoded it into. The census reads
// every table's types from the catalog. The filtered export reaches extension
// tables and custom-field columns that the bundle's member list does not name.

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// typeSamples is one non-NULL value per base type, as Postgres text. Enums and
// arrays derive theirs from the catalog, so only a new base type needs a line.
var typeSamples = map[string]string{
	"bit":         "1",
	"bool":        "true",
	"bpchar":      "A",
	"bytea":       `\x41`,
	"date":        "2026-11-18",
	"float4":      "0.25",
	"float8":      "0.92",
	"inet":        "192.0.2.1",
	"int2":        "7",
	"int4":        "7",
	"int8":        "-5000",
	"interval":    "1 day 02:00:00",
	"json":        `{"score":0.5}`,
	"jsonb":       `{"score":0.5,"ids":["0195f3a0-0000-7000-8000-000000000001"]}`,
	"numeric":     "0.92",
	"pg_snapshot": "10:20:",
	"text":        "Acme",
	"time":        "09:30:00",
	"timestamp":   "2026-11-18 09:30:00",
	"timestamptz": "2026-11-18 09:30:00+00",
	"uuid":        "0195f3a0-0000-7000-8000-000000000001",
	"varchar":     "Acme",
	"vector":      "[0.5,1]",
	"xid8":        "42",
}

// goStructCell matches what fmt.Sprint makes of a driver value with no text
// form. That is a struct, a map, a nested slice, or a uuid's raw bytes.
var goStructCell = regexp.MustCompile(`^\{[^"]*\}$|^map\[|^\[\[|^\[[0-9 ]+\]$`)

type columnType struct {
	name, schema, kind, category string
	element, elementKind         string
}

func TestEveryColumnTypeExportsAsTheValueItHolds(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	census := schemaColumnTypes(ctx, t, e.Pool)

	selects := make([]string, 0, len(census))
	columns := make([]string, 0, len(census))
	var unsampled []string
	for _, ct := range census {
		expr, ok := ct.sample()
		if !ok {
			unsampled = append(unsampled, ct.name)
			continue
		}
		selects = append(selects, expr)
		columns = append(columns, ct.name)
	}
	if len(unsampled) > 0 {
		t.Fatalf("no sample for column types %v: add one to typeSamples", unsampled)
	}

	rows, err := e.Pool.Query(ctx, "SELECT "+strings.Join(selects, ", "))
	if err != nil {
		t.Fatal(err)
	}
	data := memberData{table: "census", columns: columns}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			t.Fatal(err)
		}
		data.rows = append(data.rows, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeMemberCSV(&buf, data); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil || len(records) != 2 {
		t.Fatalf("read back %d records, %v; want a header and one row", len(records), err)
	}
	var covered []string
	for i, cell := range records[1] {
		switch {
		case cell == "":
			t.Errorf("%s: a non-NULL sample exported as an empty cell", records[0][i])
		case goStructCell.MatchString(cell):
			t.Errorf("%s: exported as a Go value %q", records[0][i], cell)
		default:
			covered = append(covered, records[0][i])
		}
	}
	assertJSONDumpHoldsNoStruct(t, data)

	want := make([]string, len(census))
	for i, ct := range census {
		want[i] = ct.name
	}
	if !slices.Equal(covered, want) {
		t.Fatalf("exported types %v, the schema holds %v", covered, want)
	}
}

// assertJSONDumpHoldsNoStruct checks the dump: a pgtype struct marshals with
// its Valid field, which no stored value carries.
func assertJSONDumpHoldsNoStruct(t *testing.T, data memberData) {
	t.Helper()
	for _, rec := range rowsAsMaps(data) {
		for column, value := range rec {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Errorf("%s: the JSON dump cannot marshal %T: %v", column, value, err)
				continue
			}
			if bytes.Contains(raw, []byte(`"Valid":`)) {
				t.Errorf("%s: the JSON dump holds a Go struct %s", column, raw)
			}
		}
	}
}

// schemaColumnTypes is every distinct type a stored column of a public table
// holds, in name order.
func schemaColumnTypes(ctx context.Context, t *testing.T, pool *pgxpool.Pool) []columnType {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT c.udt_name::text, c.udt_schema::text, t.typtype::text, t.typcategory::text,
		       coalesce(el.typname::text, ''), coalesce(el.typtype::text, '')
		  FROM information_schema.columns c
		  JOIN information_schema.tables tb
		    ON tb.table_schema = c.table_schema AND tb.table_name = c.table_name AND tb.table_type = 'BASE TABLE'
		  JOIN pg_type t ON t.typname = c.udt_name AND t.typnamespace = c.udt_schema::regnamespace
		  LEFT JOIN pg_type el ON el.oid = t.typelem AND t.typcategory = 'A'
		 WHERE c.table_schema = 'public' AND c.is_generated = 'NEVER'
		 ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	census, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (columnType, error) {
		var ct columnType
		return ct, row.Scan(&ct.name, &ct.schema, &ct.kind, &ct.category, &ct.element, &ct.elementKind)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(census) == 0 {
		t.Fatal("the catalog lists no column types; the census query reads the wrong schema")
	}
	return census
}

// sample is a SQL expression yielding one non-NULL value of the type.
func (ct columnType) sample() (string, bool) {
	if ct.category == "A" {
		element, ok := scalarSample(ct.element, ct.elementKind, ct.schema)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("ARRAY[%s, %s]", element, element), true
	}
	return scalarSample(ct.name, ct.kind, ct.schema)
}

func scalarSample(name, kind, schema string) (string, bool) {
	typeName := pgx.Identifier{name}.Sanitize()
	if kind == "e" {
		return fmt.Sprintf("(enum_range(NULL::%s))[1]", pgx.Identifier{schema, name}.Sanitize()), true
	}
	text, ok := typeSamples[name]
	if !ok {
		return "", false
	}
	return fmt.Sprintf("'%s'::%s", strings.ReplaceAll(text, "'", "''"), typeName), true
}
