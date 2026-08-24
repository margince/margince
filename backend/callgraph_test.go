// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// The call graph two censuses share: which SQL statements a function can reach
// and which functions it can call, keyed so a method is told from a plain
// function of the same name.
//
// It lives on its own because there are two callers now — the privacy census
// (which tables an erase and an anonymize each clear) and the organization
// rename census (whether every name write reaches the duplicate re-check) — and
// a graph copied for the second drifts from the first. The narrower copy then
// walks a smaller tree and says PASS, which is the failure a census cannot
// report about itself.
//
// It yields STATEMENTS rather than tables: what a statement means is the
// caller's question, and the privacy census's answer (which tables it writes)
// is not the rename census's (whether it sets a name column).

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// graphFunc is one function's reach: the statements its body can read, the
// functions it calls, and the identifiers it merely NAMES.
//
// `reads` is not redundant with `calls`. A statement held in a package-level
// var reaches the function that executes it by name alone, and a census that
// followed only calls attributed that statement to nobody.
type graphFunc struct {
	statements []string
	calls      map[string]bool
	reads      map[string]bool
}

// packageCallGraph reads one package's product files, keyed by RECEIVER TYPE
// and name, and returns the graph alongside the statements held in
// package-level `var`/`const` declarations.
//
// Bare names are not enough. `apply` is a method on one service and also a
// plausible name elsewhere; following calls by name alone once walked from the
// privacy eraser into the policy store and reported a table an erase does not
// clear. So an edge is followed only when it can be resolved: a plain function,
// or a method called on the CALLER'S OWN receiver.
//
// AN UNFOLLOWED EDGE IS NOT SAFE. A call on a stored field, an interface or a
// closure is a real limit, and the honest thing to say about it is that it can
// hide a statement from whichever caller takes that route — never that the
// route carries nothing.
func packageCallGraph(t *testing.T, dir string) (map[string]*graphFunc, map[string][]string) {
	t.Helper()
	files := parsePackageFiles(t, dir)
	held := packageLevelStatements(files)
	graph := map[string]*graphFunc{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			recvType, recvVar := receiverTypeName(fn), receiverVarName(fn)
			entry := &graphFunc{calls: map[string]bool{}, reads: map[string]bool{}}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				// Through the package's shared value reader, so a statement
				// written in double quotes with escapes decodes rather than
				// being matched as source text, and one assembled with `+` is
				// read whole.
				if expr, isExpr := node.(ast.Expr); isExpr {
					if statement, readable := stringValue(expr, nil); readable {
						entry.statements = append(entry.statements, statement)
					}
				}
				if ident, isIdent := node.(*ast.Ident); isIdent {
					entry.reads[ident.Name] = true
				}
				call, isCall := node.(*ast.CallExpr)
				if !isCall {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					entry.calls[fun.Name] = true
				case *ast.SelectorExpr:
					if base, isIdent := fun.X.(*ast.Ident); isIdent &&
						recvVar != "" && base.Name == recvVar {
						entry.calls[scrubKey(recvType, fun.Sel.Name)] = true
					}
				}
				return true
			})
			graph[scrubKey(recvType, fn.Name.Name)] = entry
		}
	}
	// A named statement counts for whoever names it, wherever it lives.
	for _, entry := range graph {
		for name := range entry.calls {
			entry.statements = append(entry.statements, held[name]...)
		}
		for name := range entry.reads {
			entry.statements = append(entry.statements, held[name]...)
		}
	}
	return graph, held
}

// parsePackageFiles reads a package's product files once.
func parsePackageFiles(t *testing.T, dir string) []*ast.File {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", path, parseErr)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatalf("no product source found under %s, so this census covered nothing", dir)
	}
	return files
}

// packageLevelStatements are the SQL statements each package-level `var`/`const`
// holds, keyed by the name that holds them.
func packageLevelStatements(files []*ast.File) map[string][]string {
	held := map[string][]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || (gen.Tok != token.VAR && gen.Tok != token.CONST) {
				continue
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				// A spec whose names and values do not align one-to-one binds no
				// statement this can read — `var a, b = twoResults()` keeps one
				// expression for two names. Skipped whole rather than by index,
				// so the second name is not silently dropped while the first is
				// read against a value that is not its own.
				if len(value.Names) != len(value.Values) {
					continue
				}
				for i, name := range value.Names {
					// Every string LITERAL in the value, not the folded whole.
					// These statements are assembled — a raw string plus a
					// helper's output — so folding them returns nothing, and a
					// reader that gave up on the fold gave up on the statement.
					ast.Inspect(value.Values[i], func(node ast.Node) bool {
						literal, isLiteral := node.(*ast.BasicLit)
						if !isLiteral || literal.Kind != token.STRING {
							return true
						}
						statement, readable := strconv.Unquote(literal.Value)
						if readable != nil {
							return true
						}
						held[name.Name] = append(held[name.Name], statement)
						return true
					})
				}
			}
		}
	}
	return held
}

// scrubKey keys a method by its receiver type and a plain function by itself.
func scrubKey(receiver, name string) string {
	if receiver == "" {
		return name
	}
	return receiver + "." + name
}

// receiverVarName is what a method calls its own receiver, so a call on it can
// be told from a call on anything else. This package's receiverTypeName already
// gives the type; only the variable was missing.
func receiverVarName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
}

// reachesFromAnyCaller reports whether the function itself, or anything that can
// call it, gets to target.
//
// The caller matters because the obligation often sits one level up: coldstart's
// column writers move a name and the duplicate re-check runs in the function
// that drove them, which is the right place for it — the re-check needs to know
// whether a name moved at all, and only the caller collecting the applied
// fields knows that.
//
// This is coarse on purpose, and the coarseness is worth stating: an ancestor
// that calls the re-check on a branch that never reaches this writer still
// counts. It is the same grain the privacy census uses, and the alternative —
// path-sensitivity — asks the graph for something these unresolvable edges
// cannot support. What it still catches is the case worth catching: a writer
// with NOTHING above it that has ever heard of the re-check.
func reachesFromAnyCaller(graph map[string]*graphFunc, name, target string) bool {
	if reaches(graph, name, target) {
		return true
	}
	for caller := range graph {
		if caller == name {
			continue
		}
		if reaches(graph, caller, name) && reaches(graph, caller, target) {
			return true
		}
	}
	return false
}

// reaches reports whether root can get to target through resolvable edges.
func reaches(graph map[string]*graphFunc, root, target string) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(name string) bool {
		if name == target {
			return true
		}
		if seen[name] {
			return false
		}
		seen[name] = true
		entry, known := graph[name]
		if !known {
			return false
		}
		for called := range entry.calls {
			if walk(called) {
				return true
			}
		}
		return false
	}
	return walk(root)
}
