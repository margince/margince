// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The gate in backend/gates holds DedupeEvidenceFields against the contract,
// but the list itself was kept by hand, and the LinkedIn and company-domain
// lanes wrote fields it never named, so the server dropped their evidence.
// This reads the writers instead: every field name an evidence snapshot can
// carry must be in the list.
func TestEveryWrittenEvidenceFieldIsListed(t *testing.T) {
	t.Parallel()
	files := packageSources(t)
	consts := stringConsts(files)
	listed := DedupeEvidenceFields()

	// Field expressions that are not constants, each with the place where the
	// value it carries is itself checked. An entry no writer uses any more fails.
	forwarded := map[string]string{
		"field":          "nearMatchEvidence's parameter: its call sites are checked",
		"r.MatchedField": "a CompanyCandidateScore field: every MatchedField value is checked",
		"right.field":    "bestCompanyNamePairing's sides, fieldDisplayName and fieldLegalName",
	}
	seen := map[string]bool{}
	for _, at := range evidenceFieldExprs(files) {
		expr := spelled(at.expr)
		if _, ok := forwarded[expr]; ok {
			seen[expr] = true
			continue
		}
		value, ok := consts[expr]
		if !ok {
			t.Errorf("%s: evidence field %s is not a string constant; name the place that checks it in forwarded", at.pos, expr)
			continue
		}
		if !slices.Contains(listed, value) {
			t.Errorf("%s: a writer files evidence field %q (%s), which DedupeEvidenceFields does not list; "+
				"the server drops that row and the pair arrives unexplained", at.pos, value, expr)
		}
	}
	for expr, why := range forwarded {
		if !seen[expr] {
			t.Errorf("forwarded entry %q (%s) matches no writer any more; remove it", expr, why)
		}
	}
	for _, field := range []string{fieldDisplayName, fieldLegalName} {
		if !slices.Contains(listed, field) {
			t.Errorf("bestCompanyNamePairing files %q, which DedupeEvidenceFields does not list", field)
		}
	}
}

type fieldExpr struct {
	pos  string
	expr ast.Expr
}

// evidenceFieldExprs finds the three places a field name enters a snapshot: the
// field key of an evidence row, nearMatchEvidence's first argument, and a
// CompanyCandidateScore's MatchedField.
func evidenceFieldExprs(files map[string]*ast.File) []fieldExpr {
	var out []fieldExpr
	for name, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				keyed := map[string]ast.Expr{}
				for _, elt := range node.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						keyed[spelled(kv.Key)] = kv.Value
					}
				}
				if value, ok := keyed["evidenceFieldKey"]; ok && keyed["evidenceSignalKey"] != nil {
					out = append(out, fieldExpr{name, value})
				}
				if value, ok := keyed["MatchedField"]; ok {
					out = append(out, fieldExpr{name, value})
				}
			case *ast.CallExpr:
				if spelled(node.Fun) == "nearMatchEvidence" && len(node.Args) > 0 {
					out = append(out, fieldExpr{name, node.Args[0]})
				}
			}
			return true
		})
	}
	return out
}

func packageSources(t *testing.T) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files[name] = file
	}
	if len(files) == 0 {
		t.Fatal("no package sources read; this test is checking nothing")
	}
	return files
}

func stringConsts(files map[string]*ast.File) map[string]string {
	consts := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					if lit, ok := value.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if text, err := strconv.Unquote(lit.Value); err == nil {
							consts[ident.Name] = text
						}
					}
				}
			}
		}
	}
	return consts
}

// spelled spells an identifier or selector the way the source does.
func spelled(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return spelled(e.X) + "." + e.Sel.Name
	default:
		return ""
	}
}
