// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// A SQL path into a jsonb column names a key the Go struct actually writes.
//
// `meeting_invitation.appointment` and `attachment_extraction.fields` hold Go structs
// that pgx marshals straight to jsonb, and SQL reads them by key:
// `appointment->>'Start'`, `f->>'Value'`. Nothing in Go connects the two. Rename a
// field or change a tag and every predicate returns NULL — no compiler error, no
// failing test, the query simply stops matching, and a scheduled reminder that never
// fires looks exactly like a quiet week.
//
// So the keys come FROM THE STRUCT here, not from a list: whatever those types
// declare is what the paths are checked against, and a field added with a tag is
// covered without this file changing.
//
// Both structs must also tag every field. Untagged, the wire format is whatever the
// field happens to be called, which is the coupling that made a rename dangerous in
// the first place — the tags are what separate the name from the key.

import (
	"go/ast"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// jsonbStoredStruct is one Go type stored as jsonb and read by SQL path.
type jsonbStoredStruct struct {
	file string // relative to the module root
	name string
	// paths matches the SQL that reads this struct's keys, capturing the key.
	paths *regexp.Regexp
	// rebuilds matches SQL that writes this column from scratch, capturing the
	// argument list. privacy's erasure replaces the whole object with the two keys
	// it keeps, so those literals write the same invariant — and a rename breaks
	// them exactly as silently as it breaks a read.
	rebuilds *regexp.Regexp
}

// The types stored as jsonb and read by SQL path. Each entry names one; the KEYS come
// from the type itself, so this list is only the question of which types are stored
// that way, not what their keys are.
var jsonbStoredStructs = []jsonbStoredStruct{{
	file:     "internal/shared/ports/connector/calendar.go",
	name:     "CalendarAppointment",
	paths:    regexp.MustCompile(`(?i)\bappointment\s*->>\s*'([^']+)'`),
	rebuilds: regexp.MustCompile(`(?is)\bappointment\s*=\s*jsonb_build_object\s*\(([^)]*)\)`),
}, {
	file: "internal/shared/ports/extraction/extraction.go",
	name: "ExtractedField",
	// `f` is what jsonb_array_elements(…fields…) is aliased to at every site.
	paths: regexp.MustCompile(`(?i)\bf\s*->>\s*'([^']+)'`),
}}

func TestASQLPathNamesAKeyItsStructWrites(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	statements := sqlStatementsUnder(t, root)
	for _, stored := range jsonbStoredStructs {
		keys := jsonKeysOf(t, root, stored)
		var paths, rebuilds int
		for _, statement := range statements {
			for _, found := range stored.paths.FindAllStringSubmatch(statement.sql, -1) {
				paths++
				if !keys[found[1]] {
					t.Errorf("%s reads %s, and %s declares no such key (%s). A path naming a key "+
						"the struct does not write returns NULL for every row, which no test and no "+
						"compiler reports",
						statement.where, found[0], stored.name, strings.Join(sortedKeys(keys), ", "))
				}
			}
		}
		for _, statement := range statements {
			if stored.rebuilds == nil {
				continue
			}
			for _, built := range stored.rebuilds.FindAllStringSubmatch(statement.sql, -1) {
				// Odd positions are values, even ones keys: jsonb_build_object takes
				// them in pairs.
				for i, argument := range strings.Split(built[1], ",") {
					key, quoted := literalKey(argument)
					if i%2 != 0 || !quoted {
						continue
					}
					rebuilds++
					if !keys[key] {
						t.Errorf("%s rebuilds appointment with key %q, and %s declares no such "+
							"key (%s). A rebuild writing a key no reader looks for loses the "+
							"value as quietly as a read naming one nobody writes",
							statement.where, key, stored.name, strings.Join(sortedKeys(keys), ", "))
					}
				}
			}
		}
		// SEPARATE floors, because one pattern must not cover for the other's
		// silence: counted together, a rebuild match alone keeps the total non-zero
		// while every read path has stopped matching, and the gate reports a clean
		// tree over statements it no longer reads.
		if paths == 0 {
			t.Errorf("no SQL path reads %s — the tree spells its keys out in "+
				"activities/scheduling_*.go and compose/dealscoutsql.go, so this gate has "+
				"stopped finding them", stored.name)
		}
		if stored.rebuilds != nil && rebuilds == 0 {
			t.Errorf("no SQL rebuilds %s, though a pattern for it is declared: "+
				"privacy/erasure_payloads.go replaces the object with jsonb_build_object, "+
				"so this has stopped finding it", stored.name)
		}
	}
}

// pathStatement is one SQL statement and where a reader will find it.
type pathStatement struct {
	where string
	sql   string
}

// sqlStatementsUnder collects the SQL of every production Go file under internal/.
//
// Through gatekit, which flattens a `+` chain: a statement assembled from several
// literals is one statement to Postgres and several fragments to the parser, and a
// path split across the join would be invisible to a per-literal reader.
func sqlStatementsUnder(t *testing.T, root string) []pathStatement {
	t.Helper()
	var found []pathStatement
	for _, path := range goSourceFiles(t, filepath.Join(root, "internal")) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("relative path of %s: %v", path, err)
		}
		for _, sql := range gatekit.SQLStatementsIn(t, path, string(body)) {
			found = append(found, pathStatement{where: rel, sql: sql})
		}
	}
	return found
}

// jsonKeysOf reads the keys a struct marshals to, and insists every field declares
// one.
func jsonKeysOf(t *testing.T, root string, stored jsonbStoredStruct) map[string]bool {
	t.Helper()
	file, err := gatekit.ParseFile(filepath.Join(root, stored.file), 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", stored.file, err)
	}
	fields := jsonbStructFields(t, file, stored)
	keys := map[string]bool{}
	for _, field := range fields {
		for _, name := range field.Names {
			key, tagged := jsonTagKey(field.Tag)
			// encoding/json skips an unexported field whatever its tag says, so it
			// contributes no key and owes no tag. A tag on one is dead text that
			// reads like a wire promise, which is worth saying.
			if !ast.IsExported(name.Name) {
				if tagged {
					t.Errorf("%s.%s is unexported, so encoding/json never writes it: the "+
						"json tag promises a key no row will carry", stored.name, name.Name)
				}
				continue
			}
			if !tagged {
				t.Errorf("%s.%s carries no json tag, so its wire key is whatever the field is "+
					"called: rename it and the SQL paths that read it return NULL. Tag it with "+
					"the key already stored", stored.name, name.Name)
				continue
			}
			keys[key] = true
		}
	}
	return keys
}

func jsonbStructFields(t *testing.T, file *ast.File, stored jsonbStoredStruct) []*ast.Field {
	t.Helper()
	var fields []*ast.Field
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != stored.name {
			return true
		}
		if declared, isStruct := spec.Type.(*ast.StructType); isStruct {
			fields = declared.Fields.List
		}
		return false
	})
	if len(fields) == 0 {
		t.Fatalf("%s declares no fields in %s: this gate reads the type for its keys, so an "+
			"empty read means the type moved or was renamed", stored.name, stored.file)
	}
	return fields
}

// jsonTagKey reads the key a field's json tag declares. A `-` is not a key: the field
// is not written, so no path can read it.
func jsonTagKey(tag *ast.BasicLit) (string, bool) {
	if tag == nil {
		return "", false
	}
	unquoted, err := strconv.Unquote(tag.Value)
	if err != nil {
		return "", false
	}
	name, _, _ := strings.Cut(reflect.StructTag(unquoted).Get("json"), ",")
	if name == "" || name == "-" {
		return "", false
	}
	return name, true
}

// literalKey reads a single-quoted SQL literal, and reports whether the argument was
// one. A column reference or an expression is not a key.
func literalKey(argument string) (string, bool) {
	trimmed := strings.TrimSpace(argument)
	if len(trimmed) < 2 || !strings.HasPrefix(trimmed, "'") || !strings.HasSuffix(trimmed, "'") {
		return "", false
	}
	return trimmed[1 : len(trimmed)-1], true
}
