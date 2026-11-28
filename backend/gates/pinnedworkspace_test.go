// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// pinnedWorkspaceFloor guards against a vacuous pass: the walker must have
// looked at at least this many production files.
const pinnedWorkspaceFloor = 1000

const databasePackage = "github.com/margince/margince/backend/internal/platform/database"

// testSupportPackages are the packages, relative to the backend root, that hold
// fixtures seeding a second workspace. They are the only non-test code allowed
// to pin a handle to a workspace.
var testSupportPackages = []string{
	"internal/compose/integration",
	"internal/modules/capture/capturetest",
	"internal/platform/testdb",
}

// acrossWorkspacesSites are the files that may build a handle needing no live
// company. Each one is cleanup that sends nothing out. A pass that sends
// messages must use ForInstallation, so it stops when the company is archived.
var acrossWorkspacesSites = []string{
	"internal/compose/jobs_storedobject.go",
	"internal/compose/jobs_suppressionjournal.go",
	"internal/compose/tracesweep/tracesweep.go",
}

// installationHandleSites may build a ForInstallation handle from a pool.
// They are the compose wiring and the programs under cmd, which inject it into
// stores, and identity's constructor from a pool. Anything deeper takes the
// handle it was given, so a test that pins its handle gets the same pin
// everywhere.
var installationHandleSites = []string{"internal/compose/", "cmd/", "internal/modules/identity/service.go"}

func mayBuildInstallationHandle(rel string) bool {
	return slices.ContainsFunc(installationHandleSites, func(site string) bool {
		return rel == site || (strings.HasSuffix(site, "/") && strings.HasPrefix(rel, site))
	})
}

// databaseUses lists every reference a file makes to name from the database
// package. It covers a call, a method value, and a use through an aliased or
// dot import.
func databaseUses(file *ast.File, name string) []token.Pos {
	var local string
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath != databasePackage {
			continue
		}
		local = "database"
		if spec.Name != nil {
			local = spec.Name.Name
		}
	}
	if local == "" || local == "_" {
		return nil
	}
	var uses []token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if pkg, ok := x.X.(*ast.Ident); ok && pkg.Name == local && x.Sel.Name == name {
				uses = append(uses, x.Pos())
			}
		case *ast.Ident:
			if local == "." && x.Name == name {
				uses = append(uses, x.Pos())
			}
		}
		return true
	})
	return uses
}

func inTestSupport(rel string) bool {
	return slices.ContainsFunc(testSupportPackages, func(dir string) bool {
		return rel == dir || strings.HasPrefix(rel, dir+"/")
	})
}

// workspaceHandleFindings judges one production file, named relative to the
// backend root.
func workspaceHandleFindings(fset *token.FileSet, file *ast.File, rel string) []string {
	if inTestSupport(rel) {
		return nil
	}
	var findings []string
	for _, pos := range databaseUses(file, "BindTo") {
		findings = append(findings, fset.Position(pos).String()+": production code uses database.BindTo. Use database.ForInstallation, or database.NewAcrossWorkspaces for cleanup that sends nothing out.")
	}
	if !mayBuildInstallationHandle(rel) {
		for _, pos := range databaseUses(file, "ForInstallation") {
			findings = append(findings, fset.Position(pos).String()+": database.ForInstallation below the compose wiring. Take the *database.DB the wiring injects, so a pinned handle stays pinned.")
		}
	}
	if !slices.Contains(acrossWorkspacesSites, rel) {
		for _, pos := range databaseUses(file, "NewAcrossWorkspaces") {
			findings = append(findings, fset.Position(pos).String()+": database.NewAcrossWorkspaces outside its cleanup sites. A pass that sends anything out must use database.ForInstallation, so it stops when the company is archived.")
		}
	}
	return findings
}

// TestNoProductionCodePinsADatabaseHandleToAWorkspace holds the resolver in
// one place. A production handle comes from database.ForInstallation, which
// looks up the installation's one workspace itself, or from
// database.NewAcrossWorkspaces at its three cleanup sites, which needs none.
// database.BindTo names a workspace by hand, so a caller outside the test
// fixtures would be a second decision about which workspace a statement runs in.
func TestNoProductionCodePinsADatabaseHandleToAWorkspace(t *testing.T) {
	t.Parallel()
	seen := 0
	usedAcross := map[string]bool{}
	for _, root := range []string{"internal", "cmd", "pkg", filepath.Join("..", "extensions")} {
		fset, files := parseGoFilesUnder(t, root)
		for _, file := range files {
			rel := path.Clean(filepath.ToSlash(fset.Position(file.Pos()).Filename))
			seen++
			for _, finding := range workspaceHandleFindings(fset, file, rel) {
				t.Error(finding)
			}
			if len(databaseUses(file, "NewAcrossWorkspaces")) > 0 {
				usedAcross[rel] = true
			}
		}
	}
	if seen < pinnedWorkspaceFloor {
		t.Fatalf("inspected only %d files, expected at least %d: the walker matched almost nothing and this gate would pass vacuously", seen, pinnedWorkspaceFloor)
	}
	for _, site := range acrossWorkspacesSites {
		if !usedAcross[site] {
			t.Errorf("%s is listed as a NewAcrossWorkspaces cleanup site but no longer calls it; remove it from acrossWorkspacesSites", site)
		}
	}
}

// TestTheWorkspaceHandleGateSeesTheSpellingsItMustRefuse feeds the gate source
// that goes around a literal database.BindTo match.
func TestTheWorkspaceHandleGateSeesTheSpellingsItMustRefuse(t *testing.T) {
	t.Parallel()
	const head = "package p\nimport %s\n"
	cases := []struct {
		name, imports, body, rel string
		want                     int
	}{
		{"the plain call", `"` + databasePackage + `"`, "var _ = database.BindTo(nil, ws)", "internal/compose/x.go", 1},
		{"an aliased import", `db "` + databasePackage + `"`, "var _ = db.BindTo(nil, ws)", "internal/compose/x.go", 1},
		{"a dot import", `. "` + databasePackage + `"`, "var _ = BindTo(nil, ws)", "internal/compose/x.go", 1},
		{"a method value", `"` + databasePackage + `"`, "var pin = database.BindTo", "internal/compose/x.go", 1},
		{"a test-support package", `"` + databasePackage + `"`, "var _ = database.BindTo(nil, ws)", "internal/compose/integration/x.go", 0},
		{"a name that only ends in a support directory", `"` + databasePackage + `"`, "var _ = database.BindTo(nil, ws)", "internal/compose/notintegration/x.go", 1},
		{"a cleanup handle outside its sites", `"` + databasePackage + `"`, "var _ = database.NewAcrossWorkspaces(nil)", "internal/compose/chatclarify/x.go", 1},
		{"a cleanup handle at its site", `"` + databasePackage + `"`, "var _ = database.NewAcrossWorkspaces(nil)", "internal/compose/jobs_storedobject.go", 0},
		{"an installation handle in the compose wiring", `"` + databasePackage + `"`, "var _ = database.ForInstallation(nil)", "internal/compose/x.go", 0},
		{"an installation handle inside a store", `"` + databasePackage + `"`, "var _ = database.ForInstallation(nil)", "internal/modules/activities/x.go", 1},
		{"an installation handle through an alias inside a platform package", `db "` + databasePackage + `"`, "var _ = db.ForInstallation(nil)", "internal/platform/originalstore/x.go", 1},
	}
	for _, tc := range cases {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, tc.rel, strings.Replace(head, "%s", tc.imports, 1)+tc.body, 0)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := workspaceHandleFindings(fset, file, tc.rel); len(got) != tc.want {
			t.Errorf("%s: %d findings, want %d: %v", tc.name, len(got), tc.want, got)
		}
	}
}
