// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// An update to a contract row is four things that must happen together: the
// guarded patch, the audit row, the contract.updated event, and the check
// violation translated into the error a caller can act on.
//
// It is the shape a write loses a half of. Spelled in two places the copies
// drift towards whichever one is edited, and the half nobody edited is where an
// audit row or an event stops being written while the domain row still lands:
// the change happened, the trail says it did not, and no consumer hears.
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
// Two neighbouring cases, and only one of them is covered elsewhere. A patch
// that files an audit row and publishes NO event is held tree-wide, by
// TestEveryAuditedMutationEmitsAnEvent. A patch that writes the contract row
// and files neither an audit row nor an event is held by NOTHING — that gate
// starts counting once it has seen an Audit call, and the module-granularity
// audit gate is already satisfied by this package's other writes. Said plainly
// because the shorter sentence read as covering both.
const updateEvent = "PublicEventContractUpdated"

// contractsPackage is where the event type is declared, and contractsDir is
// that package's directory relative to this one.
//
// The gate resolves each file's own name for the type rather than assuming one:
// a file that imported the package as `cc`, or dot-imported it, names the same
// type in a spelling a fixed comparison does not match, and a census that
// cannot see a spelling reports the writer using it as absent.
//
// The DECLARED package name is read from the source rather than taken from the
// directory. They differ here — the directory is `contracts` and the package is
// `crmcontracts` — so an unaliased import binds `crmcontracts`, and a census
// built on the directory name hunts a string no file in this tree can produce.
// That is the fail-short direction again, so the name is derived and its
// absence is fatal rather than empty.
const (
	contractsPackage = "github.com/margince/margince/backend/internal/contracts"
	contractsDir     = "../../contracts"
)

// updateEventType is how the event names itself on the wire, and the second way
// a function can publish it.
//
// storekit.Emit takes the event type as a STRING rather than deriving it from a
// payload, so a writer reaching for it publishes contract.updated without ever
// naming the Go type — invisible to a census over type names, and invisible to
// the tree-wide event-ownership gate for the same reason. One raw caller exists
// today and it is in another module, so this arm holds a route rather than a
// present defect.
const updateEventType = "contract.updated"

func TestTheContractUpdateShapeHasOneWriter(t *testing.T) {
	declared := declaredPackageName(t, contractsDir)
	emitters := functionsWhere(t, func(file *ast.File, fn *ast.FuncDecl) bool {
		return namesType(fn, localNamesFor(file, contractsPackage, declared, updateEvent)) ||
			namesLiteral(fn, updateEventType)
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

// localNamesFor renders every spelling the named type has in this file: one per
// import of its package, plus the bare name when that import is a dot import.
func localNamesFor(file *ast.File, path, declared, typeName string) []string {
	var names []string
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != path {
			continue
		}
		switch {
		case spec.Name == nil:
			// An unaliased import binds the package's DECLARED name, which is
			// not always its directory's.
			names = append(names, declared+"."+typeName)
		case spec.Name.Name == ".":
			names = append(names, typeName)
		case spec.Name.Name == "_":
			// A blank import binds no name, so the type is unreachable here.
		default:
			names = append(names, spec.Name.Name+"."+typeName)
		}
	}
	return names
}

// declaredPackageName reads the package clause of the sources in dir.
//
// Derived rather than assumed, and fatal rather than empty: a gate that
// silently resolved no name would judge a population of nothing and report a
// clean package, which is the one way a census must not fail.
func declaredPackageName(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s for its package name: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		file, perr := parser.ParseFile(fset, dir+"/"+name, nil, parser.PackageClauseOnly)
		if perr != nil || file.Name == nil {
			continue
		}
		return file.Name.Name
	}
	t.Fatalf("no package clause found in %s, so this gate cannot say what an unaliased "+
		"import of it binds and would hunt a spelling no file can produce", dir)
	return ""
}

// functionsWhere names the package's non-test functions satisfying want. The
// file is handed to want because what a type is CALLED is a fact about the file
// that names it, not about the package.
func functionsWhere(t *testing.T, want func(*ast.File, *ast.FuncDecl) bool) []string {
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
			if isFunc && fn.Body != nil && fn.Name != nil && want(file, fn) {
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

// namesLiteral reports whether fn contains the given string literal.
func namesLiteral(fn *ast.FuncDecl, want string) bool {
	if fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.BasicLit)
		if isLit && lit.Kind == token.STRING {
			if text, err := strconv.Unquote(lit.Value); err == nil && text == want {
				found = true
			}
		}
		return !found
	})
	return found
}

// namesType reports whether fn mentions the type under ANY of the spellings it
// has in the file that declares fn — in its signature or its body.
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
func namesType(fn *ast.FuncDecl, spellings []string) bool {
	if len(spellings) == 0 {
		return false
	}
	found := false
	inspect := func(node ast.Node) bool {
		if node == nil {
			return !found
		}
		expr, isExpr := node.(ast.Expr)
		if !isExpr {
			return !found
		}
		text := exprText(expr)
		for _, spelling := range spellings {
			if text == spelling {
				found = true
			}
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
