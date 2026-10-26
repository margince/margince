// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every contract request body that requires a `source` must be refused when it
// names none.
//
// encoding/json leaves an absent key at "" with no error, so a create that never
// asks stores a row with no origin. The walk finds each body with a required
// `source`, then a module function that mentions it and calls
// httperr.RequireNonBlank("source", req.Source). A body with neither fails.
// Blind spots: a function naming two bodies and guarding one passes for both,
// and a dropped error still counts.

import (
	"go/ast"
	"go/parser"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// unguardedSourceBodies are the bodies whose `source` is not an origin this gate
// can ask for by name.
var unguardedSourceBodies = gatekit.Waive(map[string]string{
	"CreateTaskRequest": "folded into CreateActivityRequest by activityOfTask, and the activity mapping " +
		"refuses a blank source; a second check here would be a second writer of the same rule",
	"WorklistPinRequest": "its `source` names the worklist lane a row was pinned from, not where a record " +
		"came from, so an origin check would refuse a pin for the wrong reason",
})

// minSourceBodies is the count of such bodies today. A contract that drops one
// lowers it in the same change, so a walk that loses bodies cannot pass.
const minSourceBodies = 15

func TestEveryContractBodyWithARequiredSourceIsRefusedWhenItNamesNone(t *testing.T) {
	t.Parallel()
	bodies := contractBodiesWithARequiredSource(t)
	if len(bodies) < minSourceBodies {
		t.Fatalf("only %d request bodies in %s require a source, want at least %d — the walk lost some",
			len(bodies), generatedContract, minSourceBodies)
	}
	guarded := sourceGuardedBodiesUnder(t, "internal/modules")
	if len(guarded) == 0 {
		t.Fatal("no function under internal/modules calls RequireNonBlank(\"source\", ...) — the walk " +
			"is reading the wrong shape")
	}
	defer unguardedSourceBodies.AssertAllMatched(t)

	for _, name := range bodies {
		if guarded[name] || unguardedSourceBodies.Waived(t, name) {
			continue
		}
		t.Errorf("%s requires a source and no function handling it calls httperr.RequireNonBlank(\"source\", ...). "+
			"An absent key decodes to \"\" and is stored as the row's origin. Refuse it in the mapping every "+
			"transport shares, or add the body to unguardedSourceBodies with the reason it is not an origin.", name)
	}
}

func TestTheSourceGuardReadsTheCallAndTheTypeInOneFunction(t *testing.T) {
	t.Parallel()
	const src = `package p

func guarded(req crmcontracts.GuardedRequest) error {
	_, err := httperr.RequireNonBlank("source", req.Source)
	return err
}

func wrongField(req crmcontracts.WrongFieldRequest) error {
	_, err := httperr.RequireNonBlank("full_name", req.Name)
	return err
}

func noCall(req crmcontracts.NoCallRequest) string { return req.Source }

func inComment(req crmcontracts.CommentedRequest) {
	// httperr.RequireNonBlank("source", req.Source)
	_ = "httperr.RequireNonBlank(\"source\", req.Source)"
}

func wrongValue(req crmcontracts.WrongValueRequest) error {
	_, err := httperr.RequireNonBlank("source", req.Title)
	return err
}

func named(req crmcontracts.NamedRequest) error {
 _, err := httperr.RequireNonBlank(sourceField, req.Source)
 return err
}

func other(req crmcontracts.OtherRequest) error {
 _, err := httperr.RequireNonBlank(otherField, req.Source)
 return err
}

func unresolved(req crmcontracts.UnresolvedRequest) error {
 _, err := httperr.RequireNonBlank(unknownField, req.Source)
 return err
}

func namedWrongValue(req crmcontracts.NamedWrongValueRequest) error {
 _, err := httperr.RequireNonBlank(sourceField, req.Title)
 return err
}

func elsewhere() error {
	_, err := httperr.RequireNonBlank("source", "x")
	return err
}
`
	file, err := parser.ParseFile(gatekit.SourceFileSet(), "probe.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	got := sourceGuardedBodies(file, map[string]string{"sourceField": "source", "otherField": "title"})
	if !got["GuardedRequest"] || !got["NamedRequest"] {
		t.Errorf("a function that calls RequireNonBlank(\"source\") and takes the body was not credited: %v", got)
	}
	for _, name := range []string{"WrongFieldRequest", "NoCallRequest", "CommentedRequest", "WrongValueRequest", "OtherRequest", "UnresolvedRequest", "NamedWrongValueRequest"} {
		if got[name] {
			t.Errorf("%s was credited without a RequireNonBlank(\"source\") call in its function", name)
		}
	}
}

// contractBodiesWithARequiredSource lists the generated request-body types that
// declare a bare string field bound to the wire name `source`, sorted.
func contractBodiesWithARequiredSource(t *testing.T) []string {
	t.Helper()
	file, err := gatekit.ParseFile(generatedContract, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", generatedContract, err)
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		structType, isStruct := spec.Type.(*ast.StructType)
		if !isStruct || !isRequestBodyName(spec.Name.Name) {
			return true
		}
		for _, field := range structType.Fields.List {
			if ident, bare := field.Type.(*ast.Ident); bare && ident.Name == "string" && jsonFieldName(field) == "source" {
				out = append(out, spec.Name.Name)
				break
			}
		}
		return true
	})
	sort.Strings(out)
	return out
}

// sourceGuardedBodiesUnder walks the hand-written, non-test Go under root.
func sourceGuardedBodiesUnder(t *testing.T, root string) map[string]bool {
	t.Helper()
	guarded := map[string]bool{}
	constants := map[string]map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return walkErr
		case entry.IsDir():
			return nil
		case !strings.HasSuffix(path, ".go"), strings.HasSuffix(path, "_test.go"), strings.HasSuffix(path, "_gen.go"):
			return nil
		}
		file, parseErr := gatekit.ParseFile(path, 0)
		if parseErr != nil {
			return parseErr
		}
		dir := filepath.Dir(path)
		if constants[dir] == nil {
			constants[dir] = gatekit.PackageStringConstants(t, dir)
		}
		for name := range sourceGuardedBodies(file, constants[dir]) {
			guarded[name] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return guarded
}

// sourceGuardedBodies names every contract type mentioned by a function that
// also calls RequireNonBlank with a source label resolved from its package.
func sourceGuardedBodies(file *ast.File, constants map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !callsRequireNonBlankSource(fn, constants) {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && isRequestBodyName(sel.Sel.Name) {
				if pkg, ok := sel.X.(*ast.Ident); ok && strings.Contains(pkg.Name, "contracts") {
					out[sel.Sel.Name] = true
				}
			}
			return true
		})
	}
	return out
}

// namesRequestSource reports a value read from a request's own Source field, so
// a guard on another field under the label "source" does not count.
func namesRequestSource(arg ast.Expr) bool {
	sel, ok := arg.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Source"
}

// callsRequireNonBlankSource reports a real call, so a comment or a string that
// spells it does not count.
func callsRequireNonBlankSource(fn *ast.FuncDecl, constants map[string]string) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "RequireNonBlank" {
			return true
		}
		if len(call.Args) == 2 && namesRequestSource(call.Args[1]) {
			if field, known := gatekit.StringExpr(call.Args[0], constants, gatekit.FoldStrict); known && field == "source" {
				found = true
			}
		}
		return true
	})
	return found
}
