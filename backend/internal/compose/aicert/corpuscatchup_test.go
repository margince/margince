// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// An agent_loop case answers catch_me_up_on from its fixture, where production
// assembles the picture in modules/search. The case cannot run that assembler
// without a database. So each section a fixture names must be one the assembler
// can emit, and a renamed section fails here, not in a paid run.
func TestEveryCatchUpFixtureNamesASectionTheAssemblerEmits(t *testing.T) {
	exact, prefixes := assemblerSectionNames(t)
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("LoadCorpus(corpus): %v", err)
	}
	checked := 0
	for _, sc := range scenarios {
		if sc.Task != string(ai.TaskAgentLoop) {
			continue
		}
		var fixture struct {
			Tools []struct {
				Tool   string `json:"tool"`
				Result struct {
					Sections []struct{ Name string } `json:"sections"`
				} `json:"result"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(sc.Fixture, &fixture); err != nil {
			t.Fatalf("agent_loop scenario %s: its fixture does not decode: %v", sc.Name, err)
		}
		for _, answer := range fixture.Tools {
			if answer.Tool != "catch_me_up_on" {
				continue
			}
			checked++
			for _, section := range answer.Result.Sections {
				if !exact[section.Name] && !hasAnyPrefix(section.Name, prefixes) {
					t.Errorf("agent_loop scenario %s: catch_me_up_on answers a section %q that modules/search never assembles (it emits %v and %v...)",
						sc.Name, section.Name, exact, prefixes)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no agent_loop fixture answers catch_me_up_on, so no section name was checked")
	}
}

// assemblerSectionNames reads every graphSection literal in modules/search: a
// constant name as itself, and a name built as "prefix" + something as a prefix.
// A name it cannot read fails the test rather than going unchecked.
func assemblerSectionNames(t *testing.T) (map[string]bool, []string) {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join("..", "..", "modules", "search", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("modules/search holds no Go files (err %v)", err)
	}
	consts := map[string]string{}
	var literals []ast.Expr
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				for i, name := range node.Names {
					if i < len(node.Values) {
						if lit, ok := node.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							value, err := strconv.Unquote(lit.Value)
							if err != nil {
								t.Fatal(err)
							}
							consts[name.Name] = value
						}
					}
				}
			case *ast.CompositeLit:
				if ident, ok := node.Type.(*ast.Ident); ok && ident.Name == "graphSection" {
					for _, elt := range node.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "name" {
								literals = append(literals, kv.Value)
							}
						}
					}
				}
			}
			return true
		})
	}
	exact := map[string]bool{}
	var prefixes []string
	for _, expr := range literals {
		switch value := expr.(type) {
		case *ast.BasicLit:
			name, err := strconv.Unquote(value.Value)
			if err != nil {
				t.Fatal(err)
			}
			exact[name] = true
		case *ast.Ident:
			name, ok := consts[value.Name]
			if !ok {
				t.Fatalf("modules/search names a section by %s, which this test cannot read", value.Name)
			}
			exact[name] = true
		case *ast.BinaryExpr:
			lit, ok := value.X.(*ast.BasicLit)
			if !ok || value.Op != token.ADD {
				t.Fatalf("modules/search builds a section name this test cannot read at %s", fset.Position(value.Pos()))
			}
			prefix, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			prefixes = append(prefixes, prefix)
		default:
			t.Fatalf("modules/search names a section in a form this test cannot read at %s", fset.Position(expr.Pos()))
		}
	}
	if !exact["recent_touches"] {
		t.Fatalf("read %v from modules/search, which lacks recent_touches; the census lost its subject", exact)
	}
	return exact, prefixes
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
