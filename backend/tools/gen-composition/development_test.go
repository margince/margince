// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func TestProductionAndDevelopmentComposeTheDeclaredUnitSets(t *testing.T) {
	previous := *includeDevelopment
	t.Cleanup(func() { *includeDevelopment = previous })
	sources, err := instanceUnits(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	examples := 0
	for _, source := range sources {
		if source.developmentOnly {
			examples++
		}
	}
	if examples == 0 {
		t.Fatal("no declared development example to exclude")
	}
	for _, development := range []bool{false, true} {
		*includeDevelopment = development
		composed, err := composedFiles(repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyUnitManifests(composed.Manifests); err != nil {
			t.Fatal(err)
		}
		actual := map[string]bool{}
		for _, manifest := range composed.Manifests {
			actual[manifest.Unit.Name] = true
		}
		for _, source := range sources {
			want := !source.developmentOnly || development
			if actual[source.name] != want {
				t.Errorf("development=%t unit=%s included=%t, want %t", development, source.name, actual[source.name], want)
			}
			if source.developmentOnly && !development {
				for path, content := range composed.Files {
					if strings.Contains(string(content), source.modulePath) {
						t.Errorf("production %s imports development module %s", path, source.modulePath)
					}
				}
			}
		}
	}
}

func TestExcludedDevelopmentModulesAreNotFetched(t *testing.T) {
	previous := *includeDevelopment
	t.Cleanup(func() { *includeDevelopment = previous })
	*includeDevelopment = false
	t.Setenv("GOPROXY", "off")
	root := t.TempDir()
	writeTree(t, root, map[string]string{"instance/go.mod": `module example.test/instance

go 1.27.3

require example.test/unavailable-demo v1.0.0 // margince:development-only
`})
	if err := fetchUnits(root); err != nil {
		t.Fatalf("production fetched an excluded example: %v", err)
	}
	*includeDevelopment = true
	if err := fetchUnits(root); err == nil || !strings.Contains(err.Error(), "example.test/unavailable-demo") {
		t.Fatalf("development fetch = %v, want the unavailable example module", err)
	}
}

func TestCompositionVerificationRefusesAnotherDevelopmentSelection(t *testing.T) {
	for _, before := range []bool{false, true} {
		if err := compareInputs(manifestInputs{IncludeDevelopment: before}, manifestInputs{IncludeDevelopment: !before}); err == nil {
			t.Fatalf("selection %t accepted composition generated with %t", !before, before)
		}
	}
}
