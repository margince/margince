// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// An update to a contract row is four things that must happen together: the
// guarded patch, the audit row, the contract.updated event, and the check
// violation translated into the error a caller can act on. Two verbs spelled
// all four out separately — a field patch and a cancellation notice — and the
// only thing that differed between the copies was the noun in the error text.
//
// That is the shape a write loses a half of. The two copies drift towards
// whichever one is edited, and the half nobody edited is where an audit row or
// an event stops being written while the domain row still lands: the change
// happened, the trail says it did not, and the consumers never hear.
//
// So the claim is that the update shape has one writer, found by the event it
// publishes.

// updateShapeOwner is the function that performs a contract update.
const updateShapeOwner = "applyContractUpdate"

// updateEvent is what an update publishes, and it is what separates this shape
// from its neighbours.
//
// The audit ACTION does not separate them: a field patch and a status
// transition both file an "update" audit row, correctly, because both update
// the row. Keying on the action would report applyStatusTx as a second copy of
// a shape it does not share — the status change publishes
// contract.status_changed and carries the from/to a consumer needs. The event
// is the discriminator, and the only one.
//
// What this gate does NOT hold: a patch that audits and publishes nothing at
// all. That has no event to be found by, and it is held tree-wide by the write
// shape gate rather than here.
const updateEvent = "PublicEventContractUpdated"

func TestTheContractUpdateShapeHasOneWriter(t *testing.T) {
	emitters := functionsWhere(t, func(fn *ast.FuncDecl) bool {
		return namesType(fn, "crmcontracts."+updateEvent)
	})
	switch {
	case len(emitters) == 0:
		t.Fatalf("nothing in this package publishes %s, so the update shape has moved and this "+
			"gate judged nothing", updateEvent)
	case len(emitters) == 1 && emitters[0] == updateShapeOwner:
	case len(emitters) == 1:
		t.Errorf("%s publishes %s, not %s — this gate is now watching the wrong function",
			emitters[0], updateEvent, updateShapeOwner)
	default:
		t.Errorf("%d functions publish %s: %s.\n\nAn update is a guarded patch, an audit row "+
			"and an event that must land together. Spelled twice they drift towards whichever "+
			"copy is edited, and the other one writes the row while the trail and the consumers "+
			"hear nothing. Go through %s.",
			len(emitters), updateEvent, strings.Join(emitters, ", "), updateShapeOwner)
	}
}

// functionsWhere names the package's non-test functions satisfying want.
func functionsWhere(t *testing.T, want func(*ast.FuncDecl) bool) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	var found []string
	files := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files++
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatalf("parsing %s: %v", name, perr)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && fn.Body != nil && fn.Name != nil && want(fn) {
				found = append(found, fn.Name.Name)
			}
		}
	}
	if files == 0 {
		t.Fatal("this package has no non-test source, so the census read nothing")
	}
	sort.Strings(found)
	return found
}

// namesType reports whether fn mentions the named type anywhere — in its
// signature or its body.
//
// A mention, not a composite literal. Under-recognition is the one direction a
// census must not fail in: a second writer that declared `var updated
// crmcontracts.PublicEventContractUpdated` and filled it in by assignment, or
// that took the payload as a parameter, contributes nothing to a literal-only
// count. The count stays at one, the gate reports PASS, and the duplicate this
// exists to catch lands unseen.
//
// Naming the type is how a function comes to publish it, and there is no way to
// publish it without naming it. The width costs nothing here: a reader of this
// event would name it too, and this module produces it rather than consuming
// it — a consumer appearing later is a thing worth failing on and looking at,
// not a false positive to design around in advance.
func namesType(fn *ast.FuncDecl, typeName string) bool {
	found := false
	inspect := func(node ast.Node) bool {
		if node == nil {
			return !found
		}
		if expr, isExpr := node.(ast.Expr); isExpr && exprText(expr) == typeName {
			found = true
		}
		return !found
	}
	ast.Inspect(fn.Type, inspect)
	if fn.Body != nil {
		ast.Inspect(fn.Body, inspect)
	}
	return found
}

func exprText(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.SelectorExpr:
		return exprText(node.X) + "." + node.Sel.Name
	case *ast.StarExpr:
		return exprText(node.X)
	}
	return ""
}
