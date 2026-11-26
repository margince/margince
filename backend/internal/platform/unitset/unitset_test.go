// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package unitset

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeInstance puts goMod at <root>/instance/go.mod and returns root.
func writeInstance(t *testing.T, goMod string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "instance"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "instance", "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func byName(units []Unit) map[string]Unit {
	out := map[string]Unit{}
	for _, u := range units {
		out[u.Name] = u
	}
	return out
}

const mixedInstance = `module i

require (
	github.com/margince/margince/backend v0.0.0
	example.test/local v0.0.0
	example.test/fetched v1.2.0
	example.test/demo v0.0.0 // margince:development-only
	example.test/helper v1.0.0 // indirect
)

replace example.test/local => ../extensions/local

replace example.test/demo => ../extensions/demo
`

func TestAProductionSelectionLeavesOutTheDevelopmentUnit(t *testing.T) {
	root := writeInstance(t, mixedInstance)
	production, err := Selected(root, false)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(production)
	if names := slices.Sorted(maps.Keys(got)); !slices.Equal(names, []string{"fetched", "local"}) {
		t.Fatalf("production units = %v, want [fetched local]: core, indirect and development-only requires are not units", names)
	}
	if u := got["local"]; u.Fetched || u.Dir != filepath.Join(root, "extensions", "local") {
		t.Errorf("local = %+v, want composed in the folder its replace names", u)
	}
	if u := got["fetched"]; !u.Fetched || u.Dir != filepath.Join(root, "build", "extensions", "fetched") {
		t.Errorf("fetched = %+v, want its copy under %s", u, FetchedDir)
	}
	if want := filepath.Join(root, "extensions", "local", ManifestFile); got["local"].ManifestPath() != want {
		t.Errorf("ManifestPath = %q, want %q", got["local"].ManifestPath(), want)
	}
}

func TestADevelopmentSelectionKeepsTheDevelopmentUnit(t *testing.T) {
	root := writeInstance(t, mixedInstance)
	development, err := Selected(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := byName(development)["demo"]; !ok || !u.DevelopmentOnly {
		t.Fatalf("development units = %+v, want demo marked development-only", development)
	}
}

func TestAnInstanceOutsideTheTreeResolvesAgainstItsOwnGoMod(t *testing.T) {
	root := t.TempDir()
	instance := filepath.Join(t.TempDir(), "client")
	if err := os.MkdirAll(instance, 0o750); err != nil {
		t.Fatal(err)
	}
	goMod := filepath.Join(instance, "go.mod")
	if err := os.WriteFile(goMod, []byte("module c\n\nrequire example.test/crm v0.0.0\n\nreplace example.test/crm => ./crm\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	units, err := Read(root, goMod)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Dir != filepath.Join(instance, "crm") {
		t.Fatalf("units = %+v, want crm in the client's own folder", units)
	}
}

func TestNoInstanceIsACoreOnlyBuild(t *testing.T) {
	units, err := Selected(t.TempDir(), false)
	if err != nil || len(units) != 0 {
		t.Fatalf("Selected = %v, %v; want no units and no error", units, err)
	}
}

func TestTheInstanceReplacesAreAppliedLikeGoApplies(t *testing.T) {
	root := writeInstance(t, `module i

require (
	example.test/crm v1.2.0
	example.test/notes v1.0.0
	example.test/tasks v1.0.0
)

replace example.test/crm v1.2.0 => example.test/crm-fork v1.2.1

replace example.test/notes v0.9.0 => ./notes

replace example.test/tasks => ./tasks
`)
	units, err := Selected(root, false)
	if err != nil {
		t.Fatal(err)
	}
	got := byName(units)
	if u := got["crm"]; !u.Fetched || u.Download != "example.test/crm-fork@v1.2.1" {
		t.Errorf("crm = %+v, want the fork's version fetched, as the version-specific replace says", u)
	}
	if u := got["notes"]; !u.Fetched || u.Download != "example.test/notes@v1.0.0" {
		t.Errorf("notes = %+v, want v1.0.0 fetched: the replace names v0.9.0, which is not the one required", u)
	}
	if u := got["tasks"]; u.Fetched || u.Dir != filepath.Join(root, "instance", "tasks") {
		t.Errorf("tasks = %+v, want the folder the every-version replace names", u)
	}
}

func TestTwoRequirementsWithOneUnitNameAreRefused(t *testing.T) {
	root := writeInstance(t, "module i\n\nrequire (\n\ta.test/crm v1.0.0\n\tb.test/crm/v2 v2.0.0\n)\n")
	if _, err := Selected(root, false); err == nil {
		t.Fatal("two requirements named crm were both accepted")
	}
}

func TestTheUnitNameIsTheModulePathsLastElement(t *testing.T) {
	for modPath, want := range map[string]string{
		"github.com/acme/margince-crm":     "margince-crm",
		"github.com/acme/crm/v2":           "crm",
		"example.com/acme/addons/greeting": "greeting",
	} {
		if got := nameFromModulePath(modPath); got != want {
			t.Errorf("nameFromModulePath(%q) = %q, want %q", modPath, got, want)
		}
	}
}
