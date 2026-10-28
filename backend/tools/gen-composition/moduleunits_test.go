// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

// writeTree writes files under dir, creating folders as needed.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// publish writes one module version into a GOPROXY=file:// tree, the layout
// the go command reads from any proxy.
func publish(t *testing.T, proxy, modPath, version string, files map[string]string) {
	t.Helper()
	src := t.TempDir()
	writeTree(t, src, files)
	at := filepath.Join(proxy, filepath.FromSlash(modPath), "@v")
	writeTree(t, at, map[string]string{
		"list":            version + "\n",
		version + ".info": `{"Version":"` + version + `"}`,
		version + ".mod":  files["go.mod"],
	})
	zf, err := os.Create(filepath.Join(at, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := zf.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	if err := modzip.CreateFromDir(zf, module.Version{Path: modPath, Version: version}, src); err != nil {
		t.Fatal(err)
	}
}

// offlineGo points the go command at a local proxy and a fresh cache, so the
// test reads no network and leaves the machine's cache alone.
func offlineGo(t *testing.T, proxy string) {
	t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxy))
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOFLAGS", "-mod=mod -modcacherw")
	t.Setenv("GOTOOLCHAIN", "local")
}

func TestAnInstallationTurnsOnTheExtensionsItBlankImports(t *testing.T) {
	proxy, root, instance := t.TempDir(), t.TempDir(), t.TempDir()
	offlineGo(t, proxy)
	publish(t, proxy, "example.test/acme/crm/v2", "v2.1.0", map[string]string{
		"go.mod": "module example.test/acme/crm/v2\n\ngo 1.21\n",
		"crm.go": "package crm\n",
	})
	publish(t, proxy, "example.test/acme/lib", "v1.0.0", map[string]string{
		"go.mod": "module example.test/acme/lib\n\ngo 1.21\n",
		"lib.go": "package lib\n\nvar X int\n",
	})
	writeTree(t, instance, map[string]string{
		"notes/go.mod":   "module example.test/acme/notes\n\ngo 1.21\n",
		"notes/notes.go": "package notes\n",
		"extensions.go": `package instance

import (
	_ "example.test/acme/crm/v2"
	_ "example.test/acme/notes"

	"example.test/acme/lib"
)

var _ = lib.X
`,
		"tools.go": "//go:build tools\n\npackage instance\n\nimport _ \"github.com/margince/margince/backend/cmd/api\"\n",
		"go.mod": `module example.test/acme/instance

go 1.21

require (
	example.test/acme/crm/v2 v2.1.0
	example.test/acme/lib v1.0.0
	example.test/acme/notes v0.0.0
)

replace example.test/acme/notes => ./notes
`,
	})

	if err := resolveModuleUnits(root, instance); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	units, err := scanExtensions(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := map[string]extensionUnit{}
	for _, u := range units {
		got[u.Name] = u
	}
	if len(got) != 2 {
		t.Fatalf("units = %v, want crm and notes, and neither the library nor core", got)
	}
	crm := got["crm"]
	if want := filepath.Join(root, "build", "extensions", "crm"); crm.Dir != want || !crm.Fetched {
		t.Errorf("crm = %s fetched=%v, want a copy at %s marked fetched", crm.Dir, crm.Fetched, want)
	}
	if err := os.WriteFile(filepath.Join(crm.Dir, "crm.go"), []byte("package crm\n"), 0o644); err != nil {
		t.Errorf("the copy must be writable, unlike the module cache: %v", err)
	}
	notes := got["notes"]
	want, err := filepath.EvalSymlinks(filepath.Join(instance, "notes"))
	if err != nil {
		t.Fatal(err)
	}
	if notes.Dir != want || notes.Fetched {
		t.Errorf("notes = %s fetched=%v, want the installation's own folder, composed in place", notes.Dir, notes.Fetched)
	}
}

func TestNoGoModClearsTheUnitsTheLastRunTurnedOn(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"build/extensions/crm/crm.go": "package crm\n",
		"build/extensions.list":       "crm\t" + filepath.Join(root, "build", "extensions", "crm") + "\n",
	})
	if err := resolveModuleUnits(root, ""); err != nil {
		t.Fatal(err)
	}
	units, err := scanExtensions(root)
	if err != nil || len(units) != 0 {
		t.Fatalf("units = %v, err = %v; want none once no installation is named", units, err)
	}
}

func TestOneNameInExtensionsAndInTheInstallationIsRefused(t *testing.T) {
	root, elsewhere := t.TempDir(), t.TempDir()
	unit := map[string]string{"go.mod": "module example.test/notes\n\ngo 1.21\n", "notes.go": "package notes\n"}
	writeUnit(t, root, "notes", unit)
	writeTree(t, elsewhere, unit)
	writeTree(t, root, map[string]string{"build/extensions.list": "notes\t" + elsewhere + "\n"})
	if _, err := scanExtensions(root); err == nil || !strings.Contains(err.Error(), `unit "notes" is both`) {
		t.Fatalf("err = %v, want the two notes units refused", err)
	}
}

func TestAFetchedUnitsStaleManifestIsRefusedNotRewritten(t *testing.T) {
	dir := t.TempDir()
	shipped := []byte(`{"name":"crm"}`)
	writeTree(t, dir, map[string]string{unitManifestFile: string(shipped)})
	err := generateUnitManifests([]derivedManifest{{
		Unit:    extensionUnit{Name: "crm", Dir: dir, Fetched: true},
		Encoded: []byte(`{"name":"crm","tools":[]}`),
	}})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v, want the stale release refused", err)
	}
	now, err := os.ReadFile(filepath.Join(dir, unitManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(now) != string(shipped) {
		t.Errorf("manifest = %s, want the shipped one left as released", now)
	}
}

func TestTheUnitNameIsTheModulePathsLastElement(t *testing.T) {
	for modPath, want := range map[string]string{
		"github.com/acme/margince-crm":     "margince-crm",
		"github.com/acme/crm/v2":           "crm",
		"example.com/acme/addons/greeting": "greeting",
	} {
		if got := unitNameFromModulePath(modPath); got != want {
			t.Errorf("unitNameFromModulePath(%q) = %q, want %q", modPath, got, want)
		}
	}
}

func TestImportingAPackageInsideAnExtensionIsRefused(t *testing.T) {
	root, instance := t.TempDir(), t.TempDir()
	writeTree(t, instance, map[string]string{
		"notes/go.mod":         "module example.test/acme/notes\n\ngo 1.21\n",
		"notes/notes.go":       "package notes\n",
		"notes/inner/inner.go": "package inner\n",
		"extensions.go":        "package instance\n\nimport _ \"example.test/acme/notes/inner\"\n",
		"go.mod":               "module example.test/acme/instance\n\ngo 1.21\n\nrequire example.test/acme/notes v0.0.0\n\nreplace example.test/acme/notes => ./notes\n",
	})
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOFLAGS", "-mod=mod")
	err := resolveModuleUnits(root, instance)
	if err == nil || !strings.Contains(err.Error(), "import the extension's module root") {
		t.Fatalf("err = %v, want the inner package refused", err)
	}
}
