// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A tool whose tier is resolved by READING a record carries that reading's
// version into its write.
//
// The dynamic tier is a verdict about a record as it was: the resolver reads it,
// the gate admits or refuses on that reading, and the read commits before the
// write opens. An agent controls both sides of that window — its own 🟢 tools can
// commit inside it — so a write that names no version lands on a row the verdict
// never described. `pinForWrite` is the one place that decides what a write is
// conditioned on: the caller's own pin, the version a released approval was
// granted against, or the version the gate read. Whichever it is, the store then
// re-checks it under the row lock, which is where the window actually closes.
//
// The corpus is derived from the RESOLVER rather than listed: a tool declares
// `ResolverInput` exactly when its tier comes from a read, so a fourth one added
// later is subject by construction rather than by somebody remembering. The
// three today — relink_activity, progress_deal, advance_deal — all pass, which is
// why this is a fence around a property that holds rather than a report of a
// break.
//
// What it cannot see: that the pin reaches the store unchanged, and that the
// store compares it under a lock. Those are one type-checked argument and
// relinkbatch.go's own tests, not something a source walk can add to.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// The method a tool declares when its tier is decided from a record read.
	tierResolver = "ResolverInput"
	// The method that performs the call once a tier admits it.
	toolHandle = "Handle"
	// The one place a write's version condition is decided.
	writePin = "pinForWrite"
)

func TestEveryReadResolvedToolPinsItsWrite(t *testing.T) {
	t.Parallel()

	resolvers, handlers := agentToolMethods(t)
	if len(resolvers) == 0 {
		t.Fatal("no tool declares " + tierResolver + " — either the resolver hook was renamed and " +
			"this gate now reads nothing, or the scan is looking in the wrong tree. A census that " +
			"finds no subject cannot report a pass.")
	}
	for _, tool := range sortedKeys(resolvers) {
		body, handled := handlers[tool]
		if !handled {
			t.Errorf("%s resolves a tier from a record read and declares no %s: the reading is taken "+
				"and nothing consumes it.", tool, toolHandle)
			continue
		}
		if !strings.Contains(body, writePin+"(") {
			t.Errorf("%s resolves its tier by reading the record and writes without %s, so its write "+
				"names no version. The resolver's verdict describes the record as it was; a write with "+
				"nothing to condition it on lands on whatever the row has become, and the agent controls "+
				"both sides of that window.", tool, writePin)
		}
	}
}

// agentToolMethods answers the receiver types declaring a tier resolver, and
// the body of every Handle keyed by the same receiver.
//
// Keyed by receiver rather than by file: a tool's two methods routinely sit
// apart, and the pairing is what the rule is about.
func agentToolMethods(t *testing.T) (resolvers map[string]bool, handlers map[string]string) {
	t.Helper()
	dir := filepath.Join(repoRoot, "backend", "internal", "modules", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	resolvers, handlers = map[string]bool{}, map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		source := string(raw)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			receiver := receiverType(fn.Recv.List[0].Type)
			if receiver == "" {
				continue
			}
			switch fn.Name.Name {
			case tierResolver:
				resolvers[receiver] = true
			case toolHandle:
				handlers[receiver] = source[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset]
			}
		}
	}
	return resolvers, handlers
}

// receiverType names the type a method hangs on, through a pointer receiver and
// through a generic one. An unrecognised shape answers "" rather than a guess:
// a census that invented a name would pair two methods that are not siblings.
func receiverType(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.StarExpr:
		return receiverType(node.X)
	case *ast.IndexExpr:
		return receiverType(node.X)
	case *ast.IndexListExpr:
		return receiverType(node.X)
	case *ast.Ident:
		return node.Name
	default:
		return ""
	}
}
