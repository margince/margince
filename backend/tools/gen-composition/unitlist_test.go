// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/platform/unitset"
)

func TestTheUnitListNamesEachComposedUnitAndItsManifest(t *testing.T) {
	previous := *includeDevelopment
	t.Cleanup(func() { *includeDevelopment = previous })
	for _, development := range []bool{false, true} {
		*includeDevelopment = development
		composed, err := composedFiles(repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		var rows []composedUnit
		if err := json.Unmarshal(composed.Files[unitListFile], &rows); err != nil {
			t.Fatalf("development=%t: %s does not decode: %v", development, unitListFile, err)
		}
		listed := map[string]bool{}
		for _, row := range rows {
			listed[row.Name] = true
			if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(row.Manifest))); err != nil {
				t.Errorf("development=%t: %s lists %s at %s, which is not there: %v", development, unitListFile, row.Name, row.Manifest, err)
			}
		}
		units, err := selectedInstanceUnits(repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != len(units) {
			t.Errorf("development=%t: %s lists %d units, the selection holds %d", development, unitListFile, len(rows), len(units))
		}
		for _, u := range units {
			if !listed[u.Name] {
				t.Errorf("development=%t: %s leaves out the composed unit %s", development, unitListFile, u.Name)
			}
		}
	}
}

func TestACoreOnlyBuildListsNoUnits(t *testing.T) {
	body, err := composedUnitList(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "[]\n" {
		t.Fatalf("%s = %q, want an empty array", unitListFile, body)
	}
}

func TestTheManifestNameIsTheUnitSetOne(t *testing.T) {
	if unitManifestFile != unitset.ManifestFile {
		t.Fatalf("gen-composition writes %q and unitset reads %q", unitManifestFile, unitset.ManifestFile)
	}
}
