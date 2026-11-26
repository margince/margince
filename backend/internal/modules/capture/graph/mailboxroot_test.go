// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// TestNoCallSpellsThePersonalMailboxRoot holds the one mailbox root. A /me
// path anywhere but personalMailbox sends a shared mailbox's call to the app's
// own identity, which has no mailbox.
func TestNoCallSpellsThePersonalMailboxRoot(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files = append(files, parsed)
	}
	// The census must see the package and the one declaration, or a pass
	// would mean the scan read nothing.
	declared, found := rootSpellings(fset, files)
	if len(files) < 10 || !declared {
		t.Fatalf("scanned %d files, declaration found %v; the scan is not reading this package", len(files), declared)
	}
	for _, spelling := range found {
		t.Errorf("%s; build the path from a.root() instead", spelling)
	}
}

// The checker itself, against planted sources: each bad form is caught, and
// the declaration, a plain use and a comparison are not.
func TestTheRootCheckerCatchesEveryPlantedSpelling(t *testing.T) {
	const decl = `const personalMailbox = "/me"` + "\n"
	for name, tc := range map[string]struct {
		source string
		want   int
	}{
		"the declaration, a use, a comparison": {decl + `var a = newHTTPAPI(personalMailbox)
var b = m == personalMailbox`, 0},
		"a literal path":       {decl + `var u = base + "/me/messages"`, 1},
		"a raw literal":        {decl + "var u = `/me/sendMail`", 1},
		"a folded literal":     {decl + `var u = base + "/" + "me" + "/messages"`, 1},
		"a relative resource":  {decl + `var r = "me/" + "messages"`, 1},
		"a constant in pieces": {decl + `const p = "/m" + "e"` + "\n" + `var u = base + p + "/x"`, 2},
		"the root as a prefix": {decl + `var u = base + personalMailbox + "/messages"`, 1},
	} {
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "planted.go", "package graph\n"+tc.source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			if declared, found := rootSpellings(fset, []*ast.File{file}); !declared || len(found) != tc.want {
				t.Errorf("declared %v, found %v; want %d", declared, found, tc.want)
			}
		})
	}
}

// rootSpellings reports whether personalMailbox is declared, and every other
// string expression that spells the personal root. Concatenations and package
// string constants are folded, so splitting the literal does not hide it.
func rootSpellings(fset *token.FileSet, files []*ast.File) (bool, []string) {
	consts := stringConstants(files)
	declared := false
	var found []string
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				if len(node.Names) == 1 && node.Names[0].Name == "personalMailbox" {
					declared = true
					return false
				}
			case *ast.BasicLit, *ast.BinaryExpr:
				text, isString := gatekit.StringExpr(node, consts, gatekit.FoldTotal)
				if !isString {
					return true
				}
				if spellsPersonalRoot(text) {
					found = append(found, fset.Position(node.Pos()).String()+" spells "+text)
				}
				return false
			}
			return true
		})
	}
	return declared, found
}

// stringConstants folds every single-name string constant, so a constant
// built from pieces, or from another constant, is read as the string it is.
func stringConstants(files []*ast.File) map[string]string {
	consts := map[string]string{}
	for range 3 {
		for _, file := range files {
			ast.Inspect(file, func(n ast.Node) bool {
				spec, ok := n.(*ast.ValueSpec)
				if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
					return true
				}
				if text, isString := gatekit.StringExpr(spec.Values[0], consts, gatekit.FoldStrict); isString {
					consts[spec.Names[0].Name] = text
				}
				return true
			})
		}
	}
	return consts
}

// spellsPersonalRoot reads each part of a folded string between the holes a
// fold leaves. It also catches the relative form Graph accepts as a
// subscription resource.
func spellsPersonalRoot(text string) bool {
	for part := range strings.SplitSeq(text, gatekit.ComputedFragment) {
		if part == personalMailbox || strings.HasPrefix(part, personalMailbox+"/") || strings.HasPrefix(part, "me/") {
			return true
		}
	}
	return false
}
