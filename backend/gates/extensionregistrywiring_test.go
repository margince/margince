// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every place that erases, exports or merges is handed the extension registry.
//
// The registry is a dependency, not a global. An eraser, export or merge store
// built without it skips every extension table, and nothing at the call site
// says so. This fails on a production build site that does not carry it.
//
// Four shapes are judged:
//   - an eraser, retention service or privacy handlers built outside the
//     privacy module needs `.WithExtensions(` on its chain;
//   - a contacts chain that carries a grant carrier (the merge wiring) needs it too;
//   - a `privacy.UntoldLead` literal needs an `Extensions` field;
//   - the plain `AssembleSAR` serves no extension table, so it must not appear.

import (
	"go/ast"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type wiringSite struct {
	path string
	line int
	what string
}

func TestEveryProductionEraserCarriesTheExtensionRegistry(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	var sites, missing []wiringSite
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		found, lacking := judgeRegistryWiring(rel, file)
		sites = append(sites, found...)
		missing = append(missing, lacking...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range missing {
		t.Errorf("%s:%d %s\n\tHand it the extension registry (compose.ExtensionData()), or its extension tables are skipped without a sign.",
			m.path, m.line, m.what)
	}
	if len(sites) < 10 {
		t.Fatalf("this census saw %d wiring sites; the tree has more than that, so the walk or a marker has gone stale", len(sites))
	}
}

// judgeRegistryWiring returns the wiring sites in one production file and the
// subset of them that lack the registry.
func judgeRegistryWiring(rel string, file *ast.File) (sites, missing []wiringSite) {
	inPrivacy := strings.HasPrefix(rel, "internal/modules/privacy/")
	inContacts := strings.HasPrefix(rel, "internal/modules/contacts/")
	chains := map[ast.Node]map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Every call of a chain reports the same constructor, each with the
		// methods above it; the union is the whole chain.
		if root, methods := chainOf(call); root != nil {
			if chains[root] == nil {
				chains[root] = map[string]bool{}
			}
			for m := range methods {
				chains[root][m] = true
			}
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CallExpr:
			name, qualifier := calleeParts(v)
			methods := chains[v]
			judge := func(what string, ok bool) {
				s := wiringSite{path: rel, line: fsetLine(v), what: what}
				sites = append(sites, s)
				if !ok {
					missing = append(missing, s)
				}
			}
			switch {
			case !inPrivacy && qualifier == "privacy" && (name == "NewEraser" || name == "NewRetentionService" || name == "NewHandlers"):
				judge("privacy."+name+" is built without .WithExtensions(...)", methods["WithExtensions"])
			case !inContacts && methods["WithGrantCarrier"]:
				judge("a merge-capable contacts chain (it carries a grant carrier) has no .WithExtensions(...)", methods["WithExtensions"])
			case !inPrivacy && qualifier == "privacy" && name == "AssembleSAR":
				judge("privacy.AssembleSAR serves no extension table; call AssembleSARWithExtensions", false)
			}
		case *ast.CompositeLit:
			if sel, ok := v.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "UntoldLead" && !inPrivacy {
				has := false
				for _, e := range v.Elts {
					if kv, ok := e.(*ast.KeyValueExpr); ok && isIdentNamed(kv.Key, "Extensions") {
						has = true
					}
				}
				s := wiringSite{path: rel, line: fsetLine(v), what: "a privacy.UntoldLead literal has no Extensions field"}
				sites = append(sites, s)
				if !has {
					missing = append(missing, s)
				}
			}
		}
		return true
	})
	return sites, missing
}

// chainOf walks down a method chain `a(...).B(...).C(...)` from a call. It
// returns the innermost call, the constructor, and the method names applied to
// it from that call upward. A call that is not a chain returns nil.
func chainOf(call *ast.CallExpr) (ast.Node, map[string]bool) {
	methods := map[string]bool{}
	var cur ast.Expr = call
	for {
		c, ok := cur.(*ast.CallExpr)
		if !ok {
			return nil, nil
		}
		sel, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return c, methods
		}
		inner, isCall := sel.X.(*ast.CallExpr)
		if !isCall {
			return c, methods
		}
		methods[sel.Sel.Name] = true
		cur = inner
	}
}

// calleeParts is the function name and the identifier qualifying it, when it is
// `pkg.Name(...)`; a bare `Name(...)` has an empty qualifier.
func calleeParts(call *ast.CallExpr) (name, qualifier string) {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name, ""
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			return f.Sel.Name, id.Name
		}
		return f.Sel.Name, ""
	}
	return "", ""
}

func fsetLine(n ast.Node) int { return gatekit.SourceFileSet().Position(n.Pos()).Line }
