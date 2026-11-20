// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The task contract's no_payload field is a data-protection control, and the
// runtime reads it in one place: Router.CapturesPayload. The control holds only
// if every lane that fills a traced call's payload asks that function first. A
// lane that sets Call.Payload on its own keeps text the contract forbids, and no
// test of the contract table sees it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// aiModuleDir is the one package that builds a traced Call: its callStore is
// the package's own interface, so no other package fills a Call's payload.
const aiModuleDir = "internal/modules/ai"

// TestEveryTracedPayloadIsGatedByTheContract fails a write of Call.Payload that
// does not sit in the body of an if whose condition asks CapturesPayload.
// Clearing a payload (assigning nil) keeps nothing and needs no guard.
func TestEveryTracedPayloadIsGatedByTheContract(t *testing.T) {
	t.Parallel()
	fset, files := parseAIModule(t)
	info, _ := typeCheckLoosely(fset, aiModuleDir, files)
	callType := aiPackageType(info, "Call")
	if callType == nil {
		t.Fatalf("no type Call declared in %s; the census has nothing to key on", aiModuleDir)
	}
	guarded := 0
	for _, file := range files {
		for _, write := range tracedPayloadWrites(file, info, callType) {
			if write.guarded {
				guarded++
				continue
			}
			t.Errorf("%s writes Call.Payload outside an `if r.CapturesPayload(task)` body: the task "+
				"contract's no_payload field would not reach this lane. Ask CapturesPayload first",
				fset.Position(write.at))
		}
	}
	if guarded == 0 {
		t.Fatalf("found no guarded Call.Payload write in %s; the walk is broken, not the lanes", aiModuleDir)
	}
}

func parseAIModule(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	entries, err := gatekit.ReadDir(aiModuleDir)
	if err != nil {
		t.Fatalf("reading %s: %v", aiModuleDir, err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(aiModuleDir, name)
		src, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files = append(files, file)
	}
	return fset, files
}

// aiPackageType is the package-level type declared as name, or nil.
func aiPackageType(info *types.Info, name string) *types.TypeName {
	for ident, obj := range info.Defs {
		if typeName, ok := obj.(*types.TypeName); ok && ident.Name == name && typeName.Parent() == typeName.Pkg().Scope() {
			return typeName
		}
	}
	return nil
}

type tracedPayloadWrite struct {
	at      token.Pos
	guarded bool
}

// tracedPayloadWrites lists every non-nil write of the Payload field of callType in
// file, by assignment or by composite literal, with whether it is guarded.
func tracedPayloadWrites(file *ast.File, info *types.Info, callType *types.TypeName) []tracedPayloadWrite {
	var writes []tracedPayloadWrite
	var path []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			path = path[:len(path)-1]
			return true
		}
		path = append(path, node)
		switch n := node.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				if isTracedPayloadField(lhs, info, callType) && i < len(n.Rhs) && !isNilLiteral(n.Rhs[i]) {
					writes = append(writes, tracedPayloadWrite{at: lhs.Pos(), guarded: underCaptureGuard(path)})
				}
			}
		case *ast.CompositeLit:
			if !isTracedCallType(info.TypeOf(n), callType) {
				return true
			}
			for _, elt := range n.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Payload" && !isNilLiteral(kv.Value) {
					writes = append(writes, tracedPayloadWrite{at: kv.Pos(), guarded: underCaptureGuard(path)})
				}
			}
		}
		return true
	})
	return writes
}

func isTracedPayloadField(expr ast.Expr, info *types.Info, callType *types.TypeName) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Payload" {
		return false
	}
	return isTracedCallType(info.TypeOf(sel.X), callType)
}

// isTracedCallType reports whether typ is callType or a pointer to it.
func isTracedCallType(typ types.Type, callType *types.TypeName) bool {
	if typ == nil {
		return false
	}
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	named, ok := typ.(*types.Named)
	return ok && named.Obj() == callType
}

func isNilLiteral(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// underCaptureGuard reports whether the innermost node of path sits in the body
// (never the else) of an if whose condition calls CapturesPayload.
func underCaptureGuard(path []ast.Node) bool {
	for i := len(path) - 2; i >= 0; i-- {
		ifStmt, ok := path[i].(*ast.IfStmt)
		if ok && path[i+1] == ifStmt.Body && callsCapturesPayload(ifStmt.Cond) {
			return true
		}
	}
	return false
}

func callsCapturesPayload(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "CapturesPayload" {
				found = true
			}
		}
		return !found
	})
	return found
}
