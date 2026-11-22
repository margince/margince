// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	repoPresets  = "../../../config/presets"
	repoFrontend = "../../../frontend/src/lib/modelsetups.gen.ts"
)

func TestTheCommittedTableIsCurrent(t *testing.T) {
	want, err := render(repoPresets)
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(repoFrontend)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(want) {
		t.Errorf("%s is stale: run make gen", repoFrontend)
	}
}

// Every preset file is in the table under its own name. A screen that names one
// by id cannot point at a preset the generator skipped.
func TestEveryPresetIsInTheTable(t *testing.T) {
	table, err := render(repoPresets)
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(repoPresets, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no preset found under %s (%v)", repoPresets, err)
	}
	for _, file := range files {
		id := strings.TrimSuffix(filepath.Base(file), ".yaml")
		if !strings.Contains(string(table), "\n  "+id+": {\n") {
			t.Errorf("preset %s is not in the table", id)
		}
	}
}

func TestADirectoryWithNoPresetIsRefused(t *testing.T) {
	if _, err := render(t.TempDir()); err == nil {
		t.Fatal("an empty presets directory rendered a table")
	}
}
