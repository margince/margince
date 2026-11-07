// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// namesOf is what Load knows when keys are every program's keys.
func namesOf(t *testing.T, roots map[string]bool, keys ...string) known {
	t.Helper()
	reg, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return knownNames(reg, Names{Keys: keys}, roots)
}

func TestYAMLFlattensToSettingKeysAndReportsUnknownOnes(t *testing.T) {
	p := writeFile(t, t.TempDir(), "margince.yaml", `
version: 1
workspace: {name: Acme}
log: {level: debug, lvl: x}
ai:
  routing: {chat: {model: m}}
`)
	roots := map[string]bool{"version": true, "workspace": true}
	layer, unknown, err := readYAML(os.ReadFile, p, namesOf(t, roots, "log.level", "ai.routing"))
	if err != nil {
		t.Fatal(err)
	}
	if layer.values["log.level"].Value != "debug" {
		t.Fatalf("log.level: %v", layer.values["log.level"])
	}
	if _, ok := layer.values["ai.routing"]; !ok {
		t.Fatal("ai.routing is a struct leaf; its node is kept whole")
	}
	if !slices.Equal(unknown, []unknownKey{{key: "log.lvl", line: 4}}) {
		t.Fatalf("unknown %v", unknown)
	}
}

func TestMissingYAMLIsAnEmptyLayer(t *testing.T) {
	layer, unknown, err := readYAML(os.ReadFile, filepath.Join(t.TempDir(), "absent.yaml"), namesOf(t, nil))
	if err != nil || len(layer.values) != 0 || len(unknown) != 0 {
		t.Fatalf("%v %v %v", layer, unknown, err)
	}
}

func TestOverlayFilesBesideTheBaseAreFound(t *testing.T) {
	dir := t.TempDir()
	base := writeFile(t, dir, "margince.yaml", "version: 1\n")
	writeFile(t, dir, "margince.dev.yaml", "version: 1\n")
	writeFile(t, dir, "margince.example.yaml", "version: 1\n")
	got := overlayBeside(base)
	if !slices.Equal(got, []string{filepath.Join(dir, "margince.dev.yaml")}) {
		t.Fatalf("got %v; example and schema files are templates, not overlays", got)
	}
}

func TestANullValueIsAKeyLeftUnwritten(t *testing.T) {
	p := writeFile(t, t.TempDir(), "margince.yaml", "log: {level: null}\nweb:\nuploads: ~\n")
	layer, unknown, err := readYAML(os.ReadFile, p, namesOf(t, nil, "log.level"))
	if err != nil {
		t.Fatal(err)
	}
	if len(layer.values) != 0 || len(unknown) != 0 {
		t.Fatalf("values %v unknown %v; a null writes nothing", layer.values, unknown)
	}
}

// A misspelt section is one fault at the line that names it. Reported per leaf,
// one typo of a parent would bury the other faults under its children.
func TestAnUnknownSectionIsReportedOnceAtItsLine(t *testing.T) {
	p := writeFile(t, t.TempDir(), "margince.yaml", "lgo:\n  level: debug\n  format: json\nlog:\n  level: info\n")
	_, unknown, err := readYAML(os.ReadFile, p, namesOf(t, nil, "log.level", "log.format"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(unknown, []unknownKey{{key: "lgo", line: 1}}) {
		t.Fatalf("unknown %v, want lgo once, at line 1", unknown)
	}
}

// A misspelling is matched against names at its own depth, the bootstrap roots
// among the top-level ones.
func TestAMisspellingIsMatchedAtItsOwnDepth(t *testing.T) {
	names := namesOf(t, map[string]bool{"workspace": true}, "log.level", "log.format", "email.smtp.host")
	for key, want := range map[string]string{
		"workspce":       "workspace",
		"lgo":            "log",
		"log.lvl":        "log.level",
		"email.smpt":     "email.smtp",
		"email.smtp.hst": "email.smtp.host",
		"redis":          "",
	} {
		if got := names.nearKey(key); got != want {
			t.Errorf("nearKey(%q) = %q, want %q", key, got, want)
		}
	}
}
