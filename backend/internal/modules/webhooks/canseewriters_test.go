// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

// One spelling of "may this subscription's owner read this record".
//
// It is asked twice — once when an event fans out, and again before every
// re-attempt of a parked delivery — and the two must agree. A second copy would
// not fail anything: the enqueue path is exercised by every fan-out test in the
// tree, so a retry copy that drifted would keep passing while quietly answering
// a different question about the same row. This is the test that fails instead.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// entityVisibleTo is the store call that IS the answer. Every path asking the
// visibility question has to reach it through canSee, which is the one place
// that resolves the owner's live RBAC and binds their principal first — calling
// it directly means asking the question as whoever the caller happens to be,
// and for a system-principal replay that is everybody.
const visibilityAnswer = "entityVisibleTo"

// canSeeCaller names the function this gate permits to make that call.
const canSeeCaller = "canSee"

func TestOnlyCanSeeAsksTheVisibilityQuestion(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package: %v", err)
	}
	// A census that reads nothing reports PASS, so the corpus is asserted
	// rather than assumed: the package has well over ten files, and a glob
	// returning two would mean the walk broke rather than that the tree shrank.
	if len(files) < 10 {
		t.Fatalf("the scan found %d files in this package, which is too few to be the whole "+
			"of it — the census is broken, not the tree", len(files))
	}
	var callers []string
	scanned := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		scanned++
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name == canSeeCaller {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != visibilityAnswer {
					return true
				}
				callers = append(callers,
					fn.Name.Name+" ("+fset.Position(sel.Pos()).String()+")")
				return true
			})
		}
	}
	if scanned == 0 {
		t.Fatal("no non-test file was scanned, so this proved nothing")
	}
	for _, caller := range callers {
		t.Errorf("%s calls %s directly, so this package now has a second answer to "+
			"\"may this owner read this record\". Route it through %s, which resolves the "+
			"owner's live RBAC and binds their principal first — asking the store directly "+
			"asks as whoever the caller happens to be, and a system-principal replay is "+
			"everybody.", caller, visibilityAnswer, canSeeCaller)
	}
}

func TestBothVisibilityAskersReachCanSee(t *testing.T) {
	// The other direction, and the one that catches a gate passing because it
	// stopped seeing its subject: the two askers must EXIST and must reach
	// canSee. Deleting the retry's call would satisfy the test above trivially.
	src, err := os.ReadFile("delivery.go")
	if err != nil {
		t.Fatalf("reading delivery.go: %v", err)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "delivery.go", src, 0)
	if err != nil {
		t.Fatalf("parsing delivery.go: %v", err)
	}
	reaches := map[string]bool{}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == canSeeCaller {
				reaches[fn.Name.Name] = true
			}
			return true
		})
	}
	for _, asker := range []string{"ownerCanSee", "stillVisible"} {
		if !reaches[asker] {
			t.Errorf("%s no longer reaches %s, so one of the two moments the visibility "+
				"question is asked has stopped asking it", asker, canSeeCaller)
		}
	}
}
