// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The integration tests here share one database per package run, and several
// rewind it or empty it, so the schema a test finds depends on the test order.
// A test that connects must therefore set its own starting schema, and every
// helper that does so drops the schema through resetSchema first.
func TestEveryTestThatConnectsSetsItsOwnStartingSchema(t *testing.T) {
	calls := callsPerFunction(t)
	if _, ok := calls["resetSchema"]; !ok {
		t.Fatal("no resetSchema in this package's tests, so this census reads nothing")
	}
	subjects := 0
	for name := range calls {
		if !strings.HasPrefix(name, "Test") || !reaches(calls, name, "connect") {
			continue
		}
		subjects++
		if !reaches(calls, name, "resetSchema") {
			t.Errorf("%s connects to the shared test database but never sets its schema; "+
				"call headSchema (or rewind it from resetSchema) first, because the test "+
				"before it in a shuffled order may have emptied or rewound it", name)
		}
	}
	if subjects == 0 {
		t.Fatal("no test here connects, so the walk is reading the wrong files")
	}
}

// callsPerFunction maps each function declared in this package's test files to
// the package-level names its body calls, closures included.
func callsPerFunction(t *testing.T) map[string]map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	calls := map[string]map[string]bool{}
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			called := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if ident, ok := call.Fun.(*ast.Ident); ok {
						called[ident.Name] = true
					}
				}
				return true
			})
			calls[fn.Name.Name] = called
		}
	}
	return calls
}

// reaches reports whether from calls target directly or through helpers.
func reaches(calls map[string]map[string]bool, from, target string) bool {
	seen := map[string]bool{}
	pending := []string{from}
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[name] {
			continue
		}
		seen[name] = true
		for callee := range calls[name] {
			if callee == target {
				return true
			}
			pending = append(pending, callee)
		}
	}
	return false
}
