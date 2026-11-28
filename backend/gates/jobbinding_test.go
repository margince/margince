// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"
)

// workMethodFloor guards against a vacuous pass. Both gates in this file
// prohibit, so finding nothing reads as success. The floor counts what the
// walker must have looked at.
const workMethodFloor = 100

// TestNoWorkerBindsItsOwnWorkspace keeps the workspace binding in one place:
// the worker middleware of the worker client. A Work body that binds on its own
// would be a second decision about which workspace a job runs in.
func TestNoWorkerBindsItsOwnWorkspace(t *testing.T) {
	t.Parallel()
	fset, files := parseGoFilesUnder(t, filepath.Join("internal", "compose"))
	seen := 0
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv == nil || fn.Name.Name != "Work" {
				continue
			}
			seen++
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := qualifiedCall(call)
				if name != "bindInstallation" && name != "principal.WithWorkspaceID" {
					return true
				}
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d: a Work method calls %s. The worker client's middleware binds the installation's workspace before any job runs.",
					pos.Filename, pos.Line, name)
				return false
			})
		}
	}
	if seen < workMethodFloor {
		t.Fatalf("inspected only %d Work methods, expected at least %d: the walker matched almost nothing and this gate would pass vacuously", seen, workMethodFloor)
	}
}

// TestEveryWorkerClientRunsTheBindingMiddleware holds the other end. River
// works jobs only on a client built with a Workers bundle, and the binding is a
// worker middleware, so it must be installed wherever such a client is built.
//
//   - River's client is constructed in exactly one file, which installs the
//     middleware in riverConfig and refuses a Config without a binding.
//   - The insert-only client never works a job, so it names no Workers and
//     needs no middleware.
//   - Every jobs.Config in production code names a BindWorkspace.
func TestEveryWorkerClientRunsTheBindingMiddleware(t *testing.T) {
	t.Parallel()
	const owner = "jobs.go"
	var clients, configs int
	for _, dir := range []string{filepath.Join("internal"), "cmd"} {
		fset, files := parseGoFilesUnder(t, dir)
		for _, file := range files {
			path := fset.Position(file.Pos()).Filename
			inOwner := strings.HasSuffix(filepath.ToSlash(path), "internal/platform/jobs/"+owner)
			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if qualifiedCall(x) == "river.NewClient" {
						clients++
						if !inOwner && mentionsKey(file, "Workers") {
							t.Errorf("%s: a River client that works jobs is built outside platform/jobs/%s, where the binding middleware is installed", fset.Position(x.Pos()), owner)
						}
					}
				case *ast.CompositeLit:
					if litTypeName(x) != "jobs.Config" {
						return true
					}
					configs++
					if !hasKey(x, "BindWorkspace") {
						t.Errorf("%s: a jobs.Config without BindWorkspace builds a worker client whose jobs run unbound", fset.Position(x.Pos()))
					}
				}
				return true
			})
			if inOwner {
				checkOwnerFile(t, file)
			}
		}
	}
	if clients < 2 || configs < 1 {
		t.Fatalf("saw %d River clients and %d jobs.Config literals, want at least 2 and 1: the walker matched almost nothing", clients, configs)
	}
}

// checkOwnerFile reads platform/jobs/jobs.go: riverConfig installs the binding,
// New refuses a missing one, and the insert-only config names no Workers.
func checkOwnerFile(t *testing.T, file *ast.File) {
	t.Helper()
	var installs, refuses bool
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.KeyValueExpr:
			if id, ok := x.Key.(*ast.Ident); ok && id.Name == "WorkerMiddleware" {
				ast.Inspect(x.Value, func(m ast.Node) bool {
					if c, ok := m.(*ast.CallExpr); ok && qualifiedCall(c) == "WorkerBinding" {
						installs = true
					}
					return true
				})
			}
		case *ast.IfStmt:
			if be, ok := x.Cond.(*ast.BinaryExpr); ok {
				if sel, ok := be.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "BindWorkspace" {
					refuses = true
				}
			}
		case *ast.FuncDecl:
			if x.Name.Name == "NewInserter" {
				ast.Inspect(x, func(m ast.Node) bool {
					if kv, ok := m.(*ast.KeyValueExpr); ok {
						if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Workers" {
							t.Errorf("NewInserter names Workers: an insert-only client must not work jobs")
						}
					}
					return true
				})
			}
		}
		return true
	})
	if !installs {
		t.Error("riverConfig does not install WorkerBinding as a WorkerMiddleware")
	}
	if !refuses {
		t.Error("jobs.New does not refuse a Config with no BindWorkspace")
	}
}

// qualifiedCall spells a call's function as pkg.Name or Name.
func qualifiedCall(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok {
			return x.Name + "." + f.Sel.Name
		}
	}
	return ""
}

// mentionsKey reports whether any composite literal in the file names key.
func mentionsKey(file *ast.File, key string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.CompositeLit); ok && hasKey(lit, key) {
			found = true
		}
		return !found
	})
	return found
}

func litTypeName(lit *ast.CompositeLit) string {
	if sel, ok := lit.Type.(*ast.SelectorExpr); ok {
		if x, ok := sel.X.(*ast.Ident); ok {
			return x.Name + "." + sel.Sel.Name
		}
	}
	return ""
}

func hasKey(lit *ast.CompositeLit, key string) bool {
	for _, e := range lit.Elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
				return true
			}
		}
	}
	return false
}
