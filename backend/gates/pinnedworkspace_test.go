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

// pinnedWorkspaceFloor guards against a vacuous pass: the walker must have
// looked at at least this many production files.
const pinnedWorkspaceFloor = 1000

// testSupportDirs hold the fixtures that seed a second workspace on purpose.
// They are the only non-test code allowed to pin a handle to a workspace.
var testSupportDirs = []string{"/integration/", "/capturetest/", "/apptest/", "/testdb/"}

// TestNoProductionCodePinsADatabaseHandleToAWorkspace holds the resolver in
// one place. A production handle comes from database.New, which finds the
// installation's one workspace itself, or from database.NewAcrossWorkspaces,
// which needs none. database.BindTo names a workspace by hand, so a caller
// outside the test fixtures would be a second decision about which workspace a
// statement runs in.
func TestNoProductionCodePinsADatabaseHandleToAWorkspace(t *testing.T) {
	t.Parallel()
	seen := 0
	for _, dir := range []string{"internal", "cmd"} {
		fset, files := parseGoFilesUnder(t, dir)
		for _, file := range files {
			path := filepath.ToSlash(fset.Position(file.Pos()).Filename)
			seen++
			if inTestSupport(path) {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if name := qualifiedCall(call); name == "database.BindTo" {
					t.Errorf("%s: production code calls database.BindTo. Use database.New, or database.NewAcrossWorkspaces for a pass that must reach an archived workspace.",
						fset.Position(call.Pos()))
				}
				return true
			})
		}
	}
	if seen < pinnedWorkspaceFloor {
		t.Fatalf("inspected only %d files, expected at least %d: the walker matched almost nothing and this gate would pass vacuously", seen, pinnedWorkspaceFloor)
	}
}

func inTestSupport(path string) bool {
	for _, dir := range testSupportDirs {
		if strings.Contains(path, dir) {
			return true
		}
	}
	return false
}
