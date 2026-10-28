// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"sort"
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

// linkInstance makes the client's folder the tree's instance/, as a client's
// make does in the core tree it puts together.
func linkInstance(t *testing.T, root, client string) {
	t.Helper()
	if err := os.Symlink(client, filepath.Join(root, instanceDirName)); err != nil {
		t.Fatal(err)
	}
}

func TestAClientInstanceComposesExactlyWhatItsGoModRequires(t *testing.T) {
	proxy, root, instance := t.TempDir(), t.TempDir(), t.TempDir()
	offlineGo(t, proxy)
	publish(t, proxy, "example.test/acme/crm/v2", "v2.1.0", map[string]string{
		"go.mod": "module example.test/acme/crm/v2\n\ngo 1.21\n\nreplace github.com/margince/margince/backend => ../../backend\n",
		"crm.go": "package crm\n",
	})
	writeTree(t, filepath.Join(root, "extensions", "unrequired"), map[string]string{"go.mod": "module example.test/unrequired\n", "u.go": "package u\n"})
	writeTree(t, instance, map[string]string{
		"notes/go.mod":   "module example.test/acme/notes\n\ngo 1.21\n",
		"notes/notes.go": "package notes\n",
		"go.mod": `module example.test/acme/instance

go 1.21

require (
	example.test/acme/crm/v2 v2.1.0
	example.test/acme/lib v1.0.0 // indirect
	example.test/acme/notes v0.0.0
	github.com/margince/margince/backend v0.0.0
	github.com/margince/margince/composition v0.0.0
)

replace example.test/acme/notes => ./notes

replace github.com/margince/margince/backend => ./not-fetched

replace github.com/margince/margince/composition => ./not-composed-yet
`,
	})

	linkInstance(t, root, instance)
	if err := fetchUnits(root); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	units, err := scanExtensions(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := map[string]string{}
	for _, u := range units {
		got[u.Name] = u.Dir
	}
	want := map[string]string{
		"crm":   filepath.Join(root, "build", "extensions", "crm"),
		"notes": filepath.Join(root, instanceDirName, "notes"),
	}
	if len(got) != len(want) || got["crm"] != want["crm"] || got["notes"] != want["notes"] {
		t.Fatalf("units = %v, want %v: not the indirect library, not core, not the unrequired folder", got, want)
	}
	copied, err := os.ReadFile(filepath.Join(got["crm"], "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(copied), "replace") {
		t.Errorf("the copy kept its replace, which Go never applies to a dependency:\n%s", copied)
	}
	if err := os.WriteFile(filepath.Join(got["crm"], "crm.go"), []byte("package crm\n"), 0o644); err != nil {
		t.Errorf("the copy must be writable, unlike the module cache: %v", err)
	}
}

func TestNoInstanceGoModComposesNothing(t *testing.T) {
	root := t.TempDir()
	unit := filepath.Join(root, "extensions", "de")
	writeTree(t, unit, map[string]string{"go.mod": "module example.test/de\n", "de.go": "package de\n"})
	units, err := scanExtensions(root)
	if err != nil || len(units) != 0 {
		t.Fatalf("units = %v, err = %v; want none, since only go.mod turns a unit on", units, err)
	}
}

func TestTwoRequirementsWithOneUnitNameAreRefused(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"instance/go.mod": "module i\n\nrequire (\n\ta.test/crm v1.0.0\n\tb.test/crm v1.0.0\n)\n"})
	if _, err := scanExtensions(root); err == nil || !strings.Contains(err.Error(), `both the unit "crm"`) {
		t.Fatalf("err = %v, want the two crm units refused", err)
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

func TestAChangedInstanceGoModRestalesTheCoreDigest(t *testing.T) {
	root := digestRoot(t)
	writeTree(t, root, map[string]string{"instance/go.mod": "module i\n\nrequire example.test/crm v1.0.0\n"})
	before, err := coreDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, map[string]string{"instance/go.mod": "module i\n\nrequire example.test/crm v1.1.0\n"})
	after, err := coreDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("a new version in instance/go.mod left the core digest unchanged, so -verify-inputs would pass a stale composition")
	}
}

// The tree's other gates read units from the folders under extensions/, so
// core's instance must turn on those and no others, by the composer's rules.
func TestCoreInstanceTurnsOnExactlyTheUnitsUnderExtensions(t *testing.T) {
	units, err := instanceUnits(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	tier, err := filepath.Abs(filepath.Join(repoRoot, "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	var required []string
	for _, u := range units {
		dir, err := filepath.Abs(u.dir)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(dir) == tier {
			required = append(required, filepath.Base(dir))
		}
	}
	entries, err := os.ReadDir(tier)
	if err != nil {
		t.Fatal(err)
	}
	var present []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(tier, e.Name(), "go.mod")); err == nil {
			present = append(present, e.Name())
		}
	}
	if len(present) == 0 {
		t.Fatal("no unit under extensions/: this test compares nothing")
	}
	sort.Strings(required)
	if strings.Join(required, " ") != strings.Join(present, " ") {
		t.Errorf("instance/go.mod turns on %v, but extensions/ holds %v; require each unit with a replace to ../extensions/<name>, or remove the folder", required, present)
	}
}

func TestUnitScreenLinksAreRelativeAndReplacedEachRun(t *testing.T) {
	root := t.TempDir()
	unit := func(name string) extensionUnit {
		dir := filepath.Join(root, "extensions", name)
		writeTree(t, dir, map[string]string{"frontend/package.json": "{}\n"})
		return extensionUnit{Name: name, Dir: dir}
	}
	if err := linkUnitScreens(root, []extensionUnit{unit("alpha"), unit("beta")}); err != nil {
		t.Fatal(err)
	}
	if err := linkUnitScreens(root, []extensionUnit{unit("beta")}); err != nil {
		t.Fatal(err)
	}
	links := filepath.Join(root, filepath.FromSlash(unitScreensDir))
	if _, err := os.Lstat(filepath.Join(links, "alpha")); !os.IsNotExist(err) {
		t.Errorf("alpha's link survived a run that no longer enables it (lstat err = %v)", err)
	}
	target, err := os.Readlink(filepath.Join(links, "beta"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("link target %q is absolute; a tree mounted elsewhere would not resolve it", target)
	}
	if _, err := os.Stat(filepath.Join(links, "beta", "package.json")); err != nil {
		t.Errorf("beta's link does not resolve to its frontend: %v", err)
	}
}

func TestADownloadedUnitClaimingAnotherModulePathIsRefused(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"instance/go.mod":               "module i\n\nrequire example.test/acme/crmx v1.2.3\n",
		"build/extensions/crmx/go.mod":  "module github.com/google/uuid\n",
		"build/extensions/crmx/crmx.go": "package uuid\n",
	})
	if _, err := scanExtensions(root); err == nil || !strings.Contains(err.Error(), `declares module "github.com/google/uuid"`) {
		t.Fatalf("err = %v, want the impostor module refused", err)
	}
}
