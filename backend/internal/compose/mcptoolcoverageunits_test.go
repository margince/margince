// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// coverageFixture writes an instance that requires a production unit and a
// development-only unit, each with a manifest declaring one agent tool.
func coverageFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"instance/go.mod": "module i\n\nrequire (\n" +
			"\texample.test/shipped v0.0.0\n" +
			"\texample.test/demo v0.0.0 // margince:development-only\n)\n\n" +
			"replace example.test/shipped => ../units/shipped\n\n" +
			"replace example.test/demo => ../units/demo\n",
		"units/shipped/manifest.generated.json": `{"name":"shipped","risk_tiers":[{"id":"tool/shipped_lookup","kind":"agent_tool"},{"id":"job/nightly","kind":"job"}]}`,
		"units/demo/manifest.generated.json":    `{"name":"demo","risk_tiers":[{"id":"tool/demo_lookup","kind":"agent_tool"}]}`,
	}
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTheToolCensusCountsOnlyTheUnitsAProductionBuildComposes(t *testing.T) {
	tools, err := extensionTools(coverageFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools {
		names = append(names, tool.Unit+"/"+tool.Name)
	}
	if !slices.Equal(names, []string{"shipped/shipped_lookup"}) {
		t.Fatalf("tools = %v, want only the shipped unit's tool: the development-only unit is not composed", names)
	}
}

func TestTheToolCensusFailsOnAComposedUnitWithNoManifest(t *testing.T) {
	root := coverageFixture(t)
	if err := os.Remove(filepath.Join(root, "units", "shipped", "manifest.generated.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := extensionTools(root); err == nil || !strings.Contains(err.Error(), "shipped") {
		t.Fatalf("err = %v, want the census to name the composed unit whose manifest is missing", err)
	}
}
