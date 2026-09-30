// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// The committed schema, read as a model rather than as text.
//
// migrations/testdata/head_catalog.txt is the projection a migration must
// update in the same commit it changes the schema, so it is the one place in
// this tree that states the whole schema without a database. Reading it here is
// what lets the entity model and its gate run in the ordinary lane: no
// Postgres, no fixture, and no second description of the tables to keep honest.
//
// The parser RECOGNISES rather than skips. Every record matches one shape or
// the walk fails, because the failure with no symptom is a line class the
// parser quietly stopped understanding: the pages would render short and read
// complete, which is the under-recognition AGENTS.md rule 8 is about.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Floors for the parse, set below today's counts so they catch a walk that
// broke rather than a schema that moved. A parser that recognised nothing would
// otherwise render an empty model and the parity gate would hold it faithfully.
const (
	catalogTableFloor  = 200
	catalogColumnFloor = 2500
)

// emColumn is one column as the catalog states it.
type emColumn struct {
	name, dataType, def string
	notNull, generated  bool
}

// emConstraint is one named constraint, definition as Postgres prints it.
type emConstraint struct{ name, def string }

// emForeignKey is a single-column reference, which is the only shape a per-column
// sentence can describe. Composite keys stay in the table's constraint list: a
// sentence on one of their columns would claim the whole key is that column.
type emForeignKey struct {
	column, parent, parentColumn, onDelete string
}

// emTable is one table's whole surface.
type emTable struct {
	name        string
	columns     []emColumn
	primaryKey  string
	checks      []emConstraint
	uniques     []emConstraint
	foreignKeys []emConstraint
	others      []emConstraint
	indexes     []emConstraint
	triggers    []emConstraint
	// singleColumnFKs is foreignKeys narrowed to the references a column
	// sentence can be built from, keyed by the referring column.
	singleColumnFKs map[string]emForeignKey
}

// emSchema holds the parsed tables and how often each is referenced. The inbound
// count is what makes a page say which records the rest of the schema hangs off.
type emSchema struct {
	tables  map[string]*emTable
	inbound map[string]int
}

// catalogRecordStart marks a line that begins a new record. Anything else
// continues the one before it: a default, a constraint or a function body can
// contain a newline, and Postgres prints it as it was written.
var catalogRecordStart = regexp.MustCompile(`^(?:public|ext)\.|^policy |^schema |^defacl `)

// The record shapes, tried in this order. Order matters where one shape's tail
// could satisfy another's: an index and a trigger both open with CREATE, and a
// constraint's definition can carry the `def=` a column line ends with.
var (
	catalogIndex      = regexp.MustCompile(`^(?:public|ext)\.([a-z0-9_]+) (CREATE (?:UNIQUE )?INDEX .*)$`)
	catalogFunction   = regexp.MustCompile(`^(?:public|ext)\.[a-z0-9_]+\([^)]*\) secdef=`)
	catalogRelation   = regexp.MustCompile(`^(?:public|ext)\.([a-z0-9_]+) acl=\S* rls=\S+ force=\S+$`)
	catalogView       = regexp.MustCompile(`^(?:public|ext)\.[a-z0-9_]+ opts=`)
	catalogTrigger    = regexp.MustCompile(`^(?:public|ext)\.([a-z0-9_]+)\.([a-zA-Z0-9_]+) (CREATE (?:CONSTRAINT )?TRIGGER .*)$`)
	catalogConstraint = regexp.MustCompile(`^(?:public|ext)\.([a-z0-9_]+)\.([a-zA-Z0-9_]+) ((?:PRIMARY KEY|FOREIGN KEY|CHECK|UNIQUE|EXCLUDE|TRIGGER) .*)$`)
	catalogColumn     = regexp.MustCompile(`^(?:public|ext)\.([a-z0-9_]+)\.([a-zA-Z0-9_]+) (.+?) gen=(\S+) def=(.*)$`)
	catalogIgnored    = regexp.MustCompile(`^(?:policy |schema |defacl )`)
	// catalogIndexTable reads which table an index is declared on, since the
	// index record names only the index.
	catalogIndexTable = regexp.MustCompile(` ON (?:public|ext)\.([a-z0-9_]+) USING `)
	// catalogForeignKey splits a reference into the parts a sentence needs.
	catalogForeignKey = regexp.MustCompile(`^FOREIGN KEY \(([^)]*)\) REFERENCES ([a-z0-9_]+)\(([^)]*)\)(.*)$`)
)

// catalogRecords joins the file's lines back into one record each.
func catalogRecords(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, headCatalogPath))
	if err != nil {
		t.Fatalf("reading %s: %v", headCatalogPath, err)
	}
	var records []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		if catalogRecordStart.MatchString(line) || len(records) == 0 {
			records = append(records, line)
			continue
		}
		records[len(records)-1] += " " + line
	}
	return records
}

// parseHeadCatalog builds the schema model, failing on any record it cannot place.
func parseHeadCatalog(t *testing.T) *emSchema {
	t.Helper()
	schema := &emSchema{tables: map[string]*emTable{}, inbound: map[string]int{}}
	var indexes []emConstraint
	var unrecognised []string
	relations := map[string]bool{}
	for _, record := range catalogRecords(t) {
		switch {
		case catalogIgnored.MatchString(record), catalogView.MatchString(record),
			catalogFunction.MatchString(record):
		case catalogRelation.MatchString(record):
			relations[catalogRelation.FindStringSubmatch(record)[1]] = true
		case catalogIndex.MatchString(record):
			m := catalogIndex.FindStringSubmatch(record)
			indexes = append(indexes, emConstraint{name: m[1], def: m[2]})
		case catalogTrigger.MatchString(record):
			m := catalogTrigger.FindStringSubmatch(record)
			tbl := schema.table(m[1])
			tbl.triggers = append(tbl.triggers, emConstraint{name: m[2], def: m[3]})
		case catalogConstraint.MatchString(record):
			m := catalogConstraint.FindStringSubmatch(record)
			schema.addConstraint(m[1], emConstraint{name: m[2], def: m[3]})
		case catalogColumn.MatchString(record):
			m := catalogColumn.FindStringSubmatch(record)
			schema.addColumn(m[1], m[2], m[3], m[4], m[5])
		default:
			unrecognised = append(unrecognised, record)
		}
	}
	for _, unknown := range unrecognised {
		t.Errorf("%s holds a record this parser does not recognise, so the entity model would "+
			"render without it and read complete:\n\t%s", headCatalogPath, truncate(unknown, 200))
	}
	schema.attachIndexes(indexes)
	schema.resolveForeignKeys()
	schema.assertEveryRelationHasColumns(t, relations)
	schema.assertNotShort(t)
	return schema
}

// table returns the named table, creating it on first sight. The catalog is
// sorted, so a table's constraints can arrive before its columns.
func (s *emSchema) table(name string) *emTable {
	if existing, ok := s.tables[name]; ok {
		return existing
	}
	created := &emTable{name: name, singleColumnFKs: map[string]emForeignKey{}}
	s.tables[name] = created
	return created
}

func (s *emSchema) addColumn(table, name, typeAndNull, generated, def string) {
	notNull := strings.HasSuffix(typeAndNull, " NOT NULL")
	tbl := s.table(table)
	tbl.columns = append(tbl.columns, emColumn{
		name:      name,
		dataType:  strings.TrimSuffix(typeAndNull, " NOT NULL"),
		def:       strings.TrimSpace(strings.TrimPrefix(def, "-")),
		notNull:   notNull,
		generated: generated != "-",
	})
}

func (s *emSchema) addConstraint(table string, con emConstraint) {
	tbl := s.table(table)
	switch {
	case strings.HasPrefix(con.def, "PRIMARY KEY"):
		tbl.primaryKey = strings.TrimSpace(strings.TrimPrefix(con.def, "PRIMARY KEY"))
	case strings.HasPrefix(con.def, "FOREIGN KEY"):
		tbl.foreignKeys = append(tbl.foreignKeys, con)
	case strings.HasPrefix(con.def, "CHECK"):
		tbl.checks = append(tbl.checks, con)
	case strings.HasPrefix(con.def, "UNIQUE"):
		tbl.uniques = append(tbl.uniques, con)
	default:
		tbl.others = append(tbl.others, con)
	}
}

// attachIndexes files each index under the table it is declared on. An index on
// a relation with no columns — a sequence — belongs to no table here.
func (s *emSchema) attachIndexes(indexes []emConstraint) {
	for _, idx := range indexes {
		on := catalogIndexTable.FindStringSubmatch(idx.def)
		if on == nil {
			continue
		}
		if tbl, ok := s.tables[on[1]]; ok {
			tbl.indexes = append(tbl.indexes, idx)
		}
	}
}

// resolveForeignKeys counts inbound references and narrows the single-column
// ones a per-column sentence can be built from.
func (s *emSchema) resolveForeignKeys() {
	for _, tbl := range s.tables {
		for _, fk := range tbl.foreignKeys {
			m := catalogForeignKey.FindStringSubmatch(fk.def)
			if m == nil {
				continue
			}
			s.inbound[m[2]]++
			columns, parentColumns := splitList(m[1]), splitList(m[3])
			if len(columns) != 1 || len(parentColumns) != 1 {
				continue
			}
			tbl.singleColumnFKs[columns[0]] = emForeignKey{
				column: columns[0], parent: m[2], parentColumn: parentColumns[0],
				onDelete: onDeleteOf(m[4]),
			}
		}
	}
}

// assertEveryRelationHasColumns is the cross-check under the parse.
//
// The catalog states each relation twice — once as a grant line, once as its
// columns — and the two arms are independent, so they disagree exactly when a
// record was lost. That is the failure this file cannot otherwise see: a
// continuation line that happens to open like a new record merges two records
// into one, the merged pair still matches a pattern, nothing is unrecognised,
// and a whole table's columns are simply gone from a page that reads complete.
//
// Sequences are the one relation with a grant and no columns, so they are the
// difference this allows.
func (s *emSchema) assertEveryRelationHasColumns(t *testing.T, relations map[string]bool) {
	t.Helper()
	for name := range relations {
		if strings.HasSuffix(name, "_seq") {
			continue
		}
		if table, ok := s.tables[name]; !ok || len(table.columns) == 0 {
			t.Errorf("%s grants on %q and states no column for it — a record was lost joining "+
				"the file's lines back together, and the model is missing a table it cannot report",
				headCatalogPath, name)
		}
	}
	for _, name := range s.tableNames() {
		if !relations[name] {
			t.Errorf("%s states columns for %q and grants on no such relation — the same loss, "+
				"read from the other arm", headCatalogPath, name)
		}
	}
}

// assertNotShort refuses a model too small to be this schema.
func (s *emSchema) assertNotShort(t *testing.T) {
	t.Helper()
	columns := 0
	for _, tbl := range s.tables {
		columns += len(tbl.columns)
	}
	if len(s.tables) < catalogTableFloor || columns < catalogColumnFloor {
		t.Fatalf("parsed %d table(s) and %d column(s) from %s, below the floor of %d and %d — "+
			"the walk has stopped reading the file rather than the schema having shrunk this far",
			len(s.tables), columns, headCatalogPath, catalogTableFloor, catalogColumnFloor)
	}
}

// tableNames returns the table names in sorted order, so callers iterate the
// schema the same way on every run.
func (s *emSchema) tableNames() []string {
	names := make([]string, 0, len(s.tables))
	for name := range s.tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func onDeleteOf(rest string) string {
	for _, action := range []string{"CASCADE", "SET NULL", "SET DEFAULT", "RESTRICT"} {
		if strings.Contains(rest, "ON DELETE "+action) {
			return action
		}
	}
	return "NO ACTION"
}

func splitList(list string) []string {
	parts := strings.Split(list, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.Trim(strings.TrimSpace(part), `"`))
	}
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
