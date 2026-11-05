// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEveryContactInsertResolvesItsEmployer derives the obligation from the actual insert statements.
func TestEveryContactInsertResolvesItsEmployer(t *testing.T) {
	t.Parallel()
	writers := 0
	for _, path := range contactWriterSources(t) {
		if strings.HasSuffix(path, "_test.go") || path == "internal/compose/integration/seed.go" {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if isGenerated(path, string(body)) {
			continue
		}
		count, missing := contactEmployerWriters(t, string(body))
		writers += count
		for _, name := range missing {
			t.Errorf("%s: %s inserts a contact without checking resolveCreatedEmployer", path, name)
		}
	}
	if writers == 0 {
		t.Fatal("no contact writers inspected")
	}
}

func contactWriterSources(t *testing.T) []string {
	t.Helper()
	paths := goSourceFiles(t, ".")
	// An installation may enable no extensions; the backend census must still be nonempty.
	err := filepath.WalkDir("../extensions", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func contactEmployerWriters(t *testing.T, body string) (int, []string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "writer.go", body, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	var missing []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		inserts, resolves := false, false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if contactCopyDestination(t, node) {
				inserts = true
			}
			if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
				text, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				for _, match := range mintSite.FindAllStringSubmatch(text, -1) {
					if strings.EqualFold(match[1], "contact") {
						inserts = true
					}
				}
			}
			if checksEmployerResolution(node) {
				resolves = true
			}
			return true
		})
		if inserts {
			count++
			if !resolves {
				missing = append(missing, fn.Name.Name)
			}
		}
	}
	return count, missing
}

func TestContactEmployerCensusRejectsMissingAndNewWriters(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("internal/modules/contacts/resolvecreate.go")
	if err != nil {
		t.Fatal(err)
	}
	validCount, validMissing := contactEmployerWriters(t, string(body))
	if validCount == 0 || len(validMissing) != 0 {
		t.Fatalf("real writer census: %d, %v", validCount, validMissing)
	}
	removed := strings.ReplaceAll(string(body), "s.resolveCreatedEmployer", "s.omittedEmployer")
	if _, missing := contactEmployerWriters(t, removed); len(missing) != 1 {
		t.Fatalf("missed removed resolver: %v", missing)
	}
	added := string(body) + "\nfunc bypass() { query := `INSERT INTO\\ncontact (full_name) VALUES ('Example')`; execute(query) }"
	added = strings.ReplaceAll(added, "INTO\\ncontact", "INTO\ncontact")
	count, missing := contactEmployerWriters(t, added)
	if count != validCount+1 || len(missing) != 1 || missing[0] != "bypass" {
		t.Fatalf("missed added writer: %d, %v", count, missing)
	}
	copied := string(body) + `
func bypassCopy() { tx.CopyFrom(ctx,pgx.Identifier{"contact"},columns,rows) }`
	if _, missing := contactEmployerWriters(t, copied); len(missing) != 1 || missing[0] != "bypassCopy" {
		t.Fatalf("missed copy writer: %v", missing)
	}
}

func contactCopyDestination(t *testing.T, node ast.Node) bool {
	t.Helper()
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "CopyFrom" {
		return false
	}
	for _, arg := range call.Args {
		table, ok := arg.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, part := range table.Elts {
			name, ok := part.(*ast.BasicLit)
			if !ok || name.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(name.Value)
			if err != nil {
				t.Fatal(err)
			}
			if value == "contact" {
				return true
			}
		}
	}
	return false
}

func checksEmployerResolution(node ast.Node) bool {
	check, ok := node.(*ast.IfStmt)
	if !ok {
		return false
	}
	assignment, ok := check.Init.(*ast.AssignStmt)
	if !ok || len(assignment.Rhs) != 1 {
		return false
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "resolveCreatedEmployer" {
		return false
	}
	for _, statement := range check.Body.List {
		if _, ok := statement.(*ast.ReturnStmt); ok {
			return true
		}
	}
	return false
}
