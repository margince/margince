// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentSelectionCoversContractsScreensAndMigrations(t *testing.T) {
	previous := *includeDevelopment
	t.Cleanup(func() { *includeDevelopment = previous })
	root, _ := committedBaseRoot(t)
	writeTree(t, root, map[string]string{goWorkFile: "go 1.27.4\nuse ./backend\n"})
	writeUnit(t, root, "demo", map[string]string{
		"go.mod": unitGoMod("demo"),
		"demo.go": `package demo
import (
 "embed"
 "github.com/margince/margince/backend/pkg/extension"
)
//go:embed migrations
var sql embed.FS
func New() extension.Extension { return extension.Extension{Name:"demo", Version:"1.0.0", Description:"A test example.", Migrations:sql} }
`,
		"api/crm.yaml":                     overlayFor("demo", "$.paths['/ext/demo/ping']", "      get:\n        operationId: demoPing\n        responses:\n          '200': {description: OK}\n"),
		"migrations/0001_example.up.sql":   "SELECT 1;\n",
		"migrations/0001_example.down.sql": "SELECT 1;\n",
	})
	writeFrontendLayer(t, filepath.Join(root, "extensions", "demo"), `{"name":"@margince-ext/demo","private":true,"main":"screen.tsx","peerDependencies":{"react":"^19.0.0"}}`)
	mod := filepath.Join(root, "instance", "go.mod")
	raw, err := os.ReadFile(mod)
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, map[string]string{"instance/go.mod": strings.Replace(string(raw), " v0.0.0\n", " v0.0.0 // "+developmentOnlyMarker+"\n", 1)})
	for _, development := range []bool{true, false} {
		*includeDevelopment = development
		units, err := scanExtensions(root)
		if err != nil {
			t.Fatal(err)
		}
		if (len(units) == 1) != development {
			t.Fatalf("development=%t units=%v", development, units)
		}
		if development && !units[0].HasMigrations {
			t.Fatal("fixture lost its migration layer")
		}
		contracts, err := composedContracts(root, units)
		if err != nil {
			t.Fatal(err)
		}
		work, _, err := composedWork(root, units)
		if err != nil {
			t.Fatal(err)
		}
		for artifact, value := range map[string]struct {
			content []byte
			needle  string
		}{
			"contract":  {contracts["crm.yaml"], "/ext/demo/ping"},
			"workspace": {work, "extensions/demo"},
			"screens":   {extScreensGen(units, nil), "@margince-ext/demo"},
			"Go wiring": {extensionsGen(units, nil, nil), "example.test/ext/demo"},
		} {
			if strings.Contains(string(value.content), value.needle) != development {
				t.Errorf("development=%t artifact=%s carries wrong example selection", development, artifact)
			}
		}

		if err := linkUnitScreens(root, units); err != nil {
			t.Fatal(err)
		}
		_, err = os.Stat(filepath.Join(root, filepath.FromSlash(unitScreensDir), "demo", "package.json"))
		if development && err != nil {
			t.Fatal(err)
		}
		if !development && !os.IsNotExist(err) {
			t.Fatalf("excluded screen link survives: %v", err)
		}
	}
}
