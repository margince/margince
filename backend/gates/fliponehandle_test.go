// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The overlay flip runs on ONE workspace binding, and this is what keeps it so.
//
// It used to run on two. The import and the reconstruction took
// actingWorkspaceDB — the workspace the operator is acting in, which for a
// rebuild onto a clean instance is one the server never resolved — while the
// mode flip took an overlay.Service built over InstallationDB, the
// installation's singleton. Two handles for one operation, and if they had ever
// named different workspaces the flip would import an estate into one workspace
// and flip the other out of overlay mode. margince/margince#2561.
//
// WHY A GATE AND NOT JUST THE FIX. The divergence was not a bug anybody could
// see: every caller is HTTP-driven and identity's middleware binds the request
// context from the same resolver the installation handle uses, so the two
// agreed on every live path and no test could have caught them differing. What
// went wrong was compositional — three steps each asked for a handle
// separately, and two of them asked a different question. Re-introducing it is
// one line, and it would be as invisible as it was the first time.
//
// So the rule is shape, not behaviour: inside flip.go, nothing may reach
// InstallationDB. The lane resolves its handle once, from the acting workspace,
// and hands it down.
func TestTheOverlayFlipLaneResolvesOneWorkspaceHandle(t *testing.T) {
	t.Parallel()

	const lane = "flip.go"
	// TestMain chdirs to the backend root, so paths here are module-relative.
	path := filepath.Join("internal", "compose", lane)
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	// The POSITIVE side first, because this gate is a prohibition and "found
	// nothing" is what success looks like — so it would read green if the walk
	// silently matched no file at all, or if the lane stopped resolving a
	// handle by any route.
	//
	// Counted against NAMED ENTRY POINTS, not per function and not per file.
	// Each is finer than the last for a reason the coarser version misses:
	//
	//   - per FILE would demand one call and fail today. Two resolutions in
	//     this file is the correct shape and always was — the parity dry-run is
	//     its own operation (zero-write, no run id, no operator) and Execute is
	//     another.
	//   - per FUNCTION admits a helper. Execute resolves once, a helper it
	//     calls resolves once, both counts are one, and the operation is back
	//     to two handles — which is the defect, not a route to it.
	//
	// So resolution is a PRIVILEGE of the two operation entry points, and every
	// other function in the lane must receive the handle. That is the rule that
	// cannot be satisfied by moving the second call somewhere else.
	resolvers := map[string]bool{"Execute": true, "parityPreview": true}
	acting := map[string]int{}
	installation := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// calleeName (retentionscope_test.go) already answers this for the
			// package, and it handles a QUALIFIED call as well as a bare one —
			// which matters here even though dbhandle.go is the same package
			// today: a helper that later moves would arrive as a selector and
			// slip past a gate that only looked at identifiers, and a
			// prohibition that stops matching is one that passes.
			switch calleeName(call) {
			case "actingWorkspaceDB":
				acting[fn.Name.Name]++
			case "InstallationDB":
				installation++
			}
			return true
		})
	}

	for name := range resolvers {
		if acting[name] == 0 {
			t.Errorf("%s: %s resolves no workspace handle. It is one of the lane's operation "+
				"entry points, so if it stopped resolving one this gate was about to pass "+
				"having checked nothing — or the entry point was renamed and this list is "+
				"now describing a function that does not exist", lane, name)
		}
	}
	for fn, count := range acting {
		switch {
		case !resolvers[fn]:
			t.Errorf("%s: %s resolves the acting workspace, and only the operation entry "+
				"points (%s) may. A helper that resolves its own handle puts the operation "+
				"back on two of them however few times each function asks (#2561) — take the "+
				"handle as a parameter instead.", lane, fn, strings.Join(sortedKeys(resolvers), ", "))
		case count > 1:
			t.Errorf("%s: %s resolves the acting workspace %d times. One operation, one handle — "+
				"a second resolution inside one function is the divergence #2561 closed, not a "+
				"route to it. Resolve once and hand it down.", lane, fn, count)
		}
	}
	if installation != 0 {
		t.Errorf("%s reaches InstallationDB %d time(s). The flip resolves ONE handle, from the "+
			"acting workspace, and hands it down: a second resolution here is how the import and "+
			"the mode flip came to target different workspaces (#2561). Pass the handle the lane "+
			"already resolved.", lane, installation)
	}

	// And the field that used to hold the second answer stays gone. A run store
	// on the runner is reachable from every method without resolving anything,
	// which is exactly how two of the three lanes ended up on the installation
	// handle while the import was on the acting one.
	if strings.Contains(runnerStruct(t, file), "RunStore") {
		t.Errorf("%s puts a RunStore on flipRunner. Every lane builds one from the handle IT "+
			"resolved — a field here is a second answer available to a caller that had already "+
			"resolved a handle (#2561)", lane)
	}
}

// runnerStruct renders the flipRunner declaration, or fails: a gate that
// silently found no struct would report the field absent because it looked at
// nothing.
func runnerStruct(t *testing.T, file *ast.File) string {
	t.Helper()
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "flipRunner" {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				t.Fatal("flipRunner is no longer a struct")
			}
			var fields []string
			for _, f := range st.Fields.List {
				fields = append(fields, renderFieldType(f.Type))
			}
			return strings.Join(fields, " ")
		}
	}
	t.Fatal("no flipRunner declaration in flip.go — this gate is about that type")
	return ""
}

func renderFieldType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return renderFieldType(t.X)
	case *ast.SelectorExpr:
		return renderFieldType(t.X) + "." + t.Sel.Name
	case *ast.Ident:
		return t.Name
	default:
		return ""
	}
}
