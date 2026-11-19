// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package rowvalue

// Every column type the schema holds must leave a document as the value it
// holds, never as a Go struct the driver decoded it into. The census reads
// every table's types from the catalog, because exports reach extension tables
// and custom-field columns no list in the code names.

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// gatekit:fixture a sample value per Postgres base type, the census's input
//
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
	"jsonb":       jsonbSample,
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

// jsonbSample holds an integer above 2^53 and an ampersand, which a float64
// decode and an HTML-escaping encoder would each change.
const jsonbSample = `{"ids":["0195f3a0-0000-7000-8000-000000000001"],"n":9007199254740993,"name":"A&B"}`

// goStructCell matches what fmt.Sprint makes of a driver value with no text
// form. That is a struct, a map, a nested slice, or a uuid's raw bytes.
var goStructCell = regexp.MustCompile(`^\{[^"]*\}$|^map\[|^\[\[|^\[[0-9 ]+\]$`)

type columnType struct {
	name, schema, kind, category string
	element, elementKind         string
}

func TestEveryColumnTypeExportsAsTheValueItHolds(t *testing.T) {
	ctx := context.Background()
	pool := censusPool(ctx, t)
	census := schemaColumnTypes(ctx, t, pool)

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

	values := sampleRow(ctx, t, pool, selects)
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	record := make([]string, len(values))
	for i, value := range values {
		cell, err := CSVCell(value)
		if err != nil {
			t.Fatalf("%s: %v", columns[i], err)
		}
		record[i] = cell
	}
	if err := cw.Write(record); err != nil {
		t.Fatal(err)
	}
	cw.Flush()
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil || len(records) != 1 {
		t.Fatalf("read back %d records, %v; want one row", len(records), err)
	}
	var covered []string
	for i, cell := range records[0] {
		switch {
		case cell == "":
			t.Errorf("%s: a non-NULL sample exported as an empty cell", columns[i])
		case columns[i] == "jsonb" && cell != jsonbSample:
			t.Errorf("jsonb: exported %s, want the stored %s", cell, jsonbSample)
		case goStructCell.MatchString(cell):
			t.Errorf("%s: exported as a Go value %q", columns[i], cell)
		default:
			covered = append(covered, columns[i])
		}
	}
	assertDocumentHoldsNoStruct(t, columns, values)

	want := make([]string, len(census))
	for i, ct := range census {
		want[i] = ct.name
	}
	if !slices.Equal(covered, want) {
		t.Fatalf("exported types %v, the schema holds %v", covered, want)
	}
}

// assertDocumentHoldsNoStruct checks the JSON side: a pgtype struct marshals
// with its Valid field, which no stored value carries.
//
//craft:ignore naked-any a row's values are []any by pgx's own contract
func assertDocumentHoldsNoStruct(t *testing.T, columns []string, values []any) {
	t.Helper()
	for i, value := range values {
		raw, err := json.Marshal(Document(value))
		if err != nil {
			t.Errorf("%s: a document cannot marshal %T: %v", columns[i], value, err)
			continue
		}
		if bytes.Contains(raw, []byte(`"Valid":`)) {
			t.Errorf("%s: a document holds a Go struct %s", columns[i], raw)
		}
	}
}

// sampleRow reads one row of samples through a Reader, as every export does.
//
//craft:ignore naked-any a row's values are []any by pgx's own contract
func sampleRow(ctx context.Context, t *testing.T, pool *pgxpool.Pool, selects []string) []any {
	t.Helper()
	rows, err := pool.Query(ctx, "SELECT "+strings.Join(selects, ", "))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("the sample query returned no row: %v", rows.Err())
	}
	values, err := NewReader().Values(rows)
	if err != nil {
		t.Fatal(err)
	}
	return values
}

// censusPool is the app role's pool over a migrated schema, so values decode
// with the type map production reads with.
func censusPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.OwnPool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// schemaColumnTypes lists the distinct types the stored columns of public tables
// hold, in name order.
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
