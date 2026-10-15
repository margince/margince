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

func TestYAMLFlattensToSettingKeysAndReportsUnknownOnes(t *testing.T) {
	p := writeFile(t, t.TempDir(), "margince.yaml", `
version: 1
workspace: {name: Acme}
log: {level: debug, levle: x}
ai:
  routing: {chat: {model: m}}
`)
	known := func(k string) bool { return k == "log.level" || k == "ai.routing" }
	roots := map[string]bool{"version": true, "workspace": true}
	layer, unknown, err := readYAML(p, known, roots)
	if err != nil {
		t.Fatal(err)
	}
	if layer.values["log.level"].Value != "debug" {
		t.Fatalf("log.level: %v", layer.values["log.level"])
	}
	if _, ok := layer.values["ai.routing"]; !ok {
		t.Fatal("ai.routing is a struct leaf; its node is kept whole")
	}
	if !slices.Equal(unknown, []string{"log.levle"}) {
		t.Fatalf("unknown %v", unknown)
	}
}

func TestMissingYAMLIsAnEmptyLayer(t *testing.T) {
	layer, unknown, err := readYAML(filepath.Join(t.TempDir(), "absent.yaml"), func(string) bool { return false }, nil)
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
