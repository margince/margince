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
	repoStock    = "../../internal/modules/ai/stockpresets_gen.go"
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

// The Go table holds every committed preset as its file reads. A binding two
// presets share is then seen as shared, whichever of them orders its tasks.
func TestTheCommittedStockTableIsCurrent(t *testing.T) {
	want, err := renderStock(repoPresets)
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(repoStock)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(want) {
		t.Errorf("%s is stale: run make gen", repoStock)
	}
	if !strings.Contains(string(want), `"gemini_cloud.yaml"`) || !strings.Contains(string(want), `"openrouter_cloud.yaml"`) {
		t.Errorf("the stock table names the wrong presets:\n%s", want)
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
