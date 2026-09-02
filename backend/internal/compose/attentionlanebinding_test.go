// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A lane whose seam is written must be wired to it.
//
// The attention feed takes each optional lane as an interface, and nil is a
// legitimate value: it renders the lane ABSENT, which is the honest answer for
// an installation that genuinely cannot fill it. That makes an unwired lane
// indistinguishable from a deliberate one — the commitments lane sat nil for
// months after its writer landed, so every rep's own promises were missing from
// their queue and nothing failed.
//
// This reads the wiring itself rather than keeping a list beside it, so a lane
// that grows a seam and is never bound fails here instead of going quiet.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestEveryLaneWithASeamIsWiredToIt fails when newAttentionService passes nil
// for a lane whose seam type exists in this package.
//
// The corpus is derived from the seam types themselves — every `attention<Name>`
// struct in this package that satisfies a lane interface — so a new lane joins
// the check by being written, not by being remembered.
func TestEveryLaneWithASeamIsWiredToIt(t *testing.T) {
	files := composeSourceFiles(t)
	seams := seamTypeNames(files)
	if len(seams) == 0 {
		t.Fatal("found no attention seam types at all — the scan is looking in the wrong place, " +
			"which would report PASS on a tree where every lane was unwired")
	}
	wired := wiredSeamNames(t, files)

	for _, seam := range seams {
		if !wired[seam] {
			t.Errorf("%s is written but newAttentionService does not pass it. A lane left nil "+
				"renders ABSENT, so the rows it would serve are silently missing rather than "+
				"reported as withheld. Wire it, or say in newAttentionService why this "+
				"installation cannot fill it.", seam)
		}
	}
}

// composeSourceFiles parses the Go files of this package, keyed by file name so
// a caller can tell a test file from production wiring.
func composeSourceFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the compose package directory: %v", err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", name, parseErr)
		}
		files[name] = parsed
	}
	if len(files) == 0 {
		t.Fatal("the compose package parsed to nothing; this test reads its own wiring")
	}
	return files
}

// seamTypeNames collects the `attention<Name>` structs declared in this package,
// skipping test files so a fixture cannot pose as a seam.
func seamTypeNames(files map[string]*ast.File) []string {
	var out []string
	for path, file := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok || !strings.HasPrefix(spec.Name.Name, "attention") {
				return true
			}
			if _, isStruct := spec.Type.(*ast.StructType); isStruct {
				out = append(out, spec.Name.Name)
			}
			return true
		})
	}
	return out
}

// wiredSeamNames collects the struct types passed as ARGUMENTS to a binding
// call inside newAttentionService.
//
// Two calls bind a lane: attention.NewService takes the required ones directly,
// and the optional ones arrive through a With<Lane> method chained off it.
// Arguments only, never any literal in the function body — a seam constructed
// into an unused variable is not a lane the service received, and counting it
// would let a lane be built beside the nil that was passed in its place, which
// is the exact defect this file exists to catch.
func wiredSeamNames(t *testing.T, files map[string]*ast.File) map[string]bool {
	t.Helper()
	wired := map[string]bool{}
	found := false
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "newAttentionService" || fn.Body == nil {
				return true
			}
			found = true
			ast.Inspect(fn.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok || !bindsALane(call.Fun) {
					return true
				}
				for _, arg := range call.Args {
					lit, ok := arg.(*ast.CompositeLit)
					if !ok {
						continue
					}
					if name, ok := lit.Type.(*ast.Ident); ok {
						wired[name.Name] = true
					}
				}
				return true
			})
			return false
		})
	}
	if !found {
		t.Fatal("newAttentionService was not found; the scan would report every lane unwired")
	}
	if len(wired) == 0 {
		t.Fatal("no lane was read out of the binding calls — attention.NewService was renamed " +
			"or reshaped, and this scan would report every lane unwired")
	}
	return wired
}

// bindsALane recognises the two calls that hand a seam to the feed:
// attention.NewService and any With<Lane> method chained off it.
func bindsALane(fun ast.Expr) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if strings.HasPrefix(sel.Sel.Name, "With") {
		return true
	}
	if sel.Sel.Name != "NewService" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "attention"
}
