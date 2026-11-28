// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"path/filepath"
	"testing"
)

// workspaceBindFloor guards against a vacuous pass. This gate is a
// PROHIBITION, so "found nothing" is what success looks like, which means it
// would also read green if the walker silently matched no files at all. The
// floor counts the POSITIVE side instead: Work methods that bind through the
// installation helpers.
const workspaceBindFloor = 15

// TestOnlyTheSharedHelperBindsAWorkspace holds a job's workspace binding in one
// place. A job names no workspace, so a Work body binds the installation's
// through bindInstallation or installationJobCtx, and this gate keeps a Work
// body from binding inline beside them.
func TestOnlyTheSharedHelperBindsAWorkspace(t *testing.T) {
	t.Parallel()
	fset, files := parseGoFilesUnder(t, filepath.Join("internal", "compose"))
	guarded := 0
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fn.Recv == nil || fn.Name.Name != "Work" {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && (id.Name == "bindInstallation" || id.Name == "installationJobCtx") {
					guarded++
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "principal" || sel.Sel.Name != "WithWorkspaceID" {
					return true
				}
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d: a Work method binds its own workspace. Bind through bindInstallation or installationJobCtx, which resolve the installation's one workspace.",
					pos.Filename, pos.Line)
				return false
			})
		}
	}
	if guarded < workspaceBindFloor {
		t.Fatalf("only %d Work methods reach the installation binders, expected at least %d — the walker matched almost nothing and this prohibition would pass vacuously",
			guarded, workspaceBindFloor)
	}
}
