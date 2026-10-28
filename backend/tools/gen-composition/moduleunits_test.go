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

func TestAnInstallationTurnsOnTheExtensionsItsGoModRequires(t *testing.T) {
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
		"notes/frontend/node_modules/react/index.js": "module.exports = {}\n",
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
		t.Fatalf("units = %v, want crm and notes, and neither the indirect library nor core", got)
	}
	if err := os.WriteFile(filepath.Join(got["crm"].Dir, "crm.go"), []byte("package crm\n"), 0o644); err != nil {
		t.Errorf("the copy must be writable, unlike the module cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(got["notes"].Dir, "node_modules")); !os.IsNotExist(err) {
		t.Errorf("a local unit's installed node_modules was copied (stat err = %v)", err)
	}
}

func TestTheNextRunRemovesOnlyTheUnitsItCopied(t *testing.T) {
	root := t.TempDir()
	writeUnit(t, root, "de", map[string]string{"go.mod": "module example.test/de\n\ngo 1.21\n", "de.go": "package de\n"})
	writeUnit(t, root, "crm", map[string]string{"go.mod": "module example.test/crm\n\ngo 1.21\n", "crm.go": "package crm\n"})
	writeTree(t, root, map[string]string{"build/extensions.list": "crm\n"})
	if err := resolveModuleUnits(root, ""); err != nil {
		t.Fatal(err)
	}
	units, err := scanExtensions(root)
	if err != nil || len(units) != 1 || units[0].Name != "de" {
		t.Fatalf("units = %v, err = %v; want only the tree's own de once no installation is named", units, err)
	}
}

func TestAnImportNamedLikeAUnitOfTheTreeIsRefused(t *testing.T) {
	root, instance := t.TempDir(), t.TempDir()
	writeUnit(t, root, "notes", map[string]string{"go.mod": "module example.test/notes\n\ngo 1.21\n", "notes.go": "package notes\n"})
	writeTree(t, instance, map[string]string{
		"notes/go.mod":   "module example.test/acme/notes\n\ngo 1.21\n",
		"notes/notes.go": "package notes\n",
		"go.mod":         "module example.test/acme/instance\n\ngo 1.21\n\nrequire example.test/acme/notes v0.0.0\n\nreplace example.test/acme/notes => ./notes\n",
	})
	t.Setenv("GOTOOLCHAIN", "local")
	err := resolveModuleUnits(root, instance)
	if err == nil || !strings.Contains(err.Error(), "extensions/notes already exists") {
		t.Fatalf("err = %v, want the second notes refused", err)
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
