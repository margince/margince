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
	repoCatalog  = "../../api/worklist-kinds.yaml"
	repoLocales  = "../../../frontend/src/i18n"
	repoGoTable  = "../../internal/compose/attention/kinds_gen.go"
	repoFrontend = "../../../frontend/src/lib/worklistkinds.gen.ts"
)

func TestTheCommittedTablesAreCurrent(t *testing.T) {
	c, err := loadCatalog(repoCatalog)
	if err != nil {
		t.Fatal(err)
	}
	goTable, err := emitGo(c)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]byte{repoGoTable: goTable, repoFrontend: emitFrontend(c)} {
		onDisk, err := os.ReadFile(path) //nolint:gosec // a committed file of this tree
		if err != nil {
			t.Fatal(err)
		}
		if string(onDisk) != string(want) {
			t.Errorf("%s is stale: run make gen", path)
		}
	}
}

func TestEveryKindIsTitledInEveryLocale(t *testing.T) {
	c, err := loadCatalog(repoCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkTitles(c, repoLocales); err != nil {
		t.Error(err)
	}
}

func TestAKindWithNoTitleIsRefused(t *testing.T) {
	c, err := parseCatalog([]byte(validKind))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, file := range locales {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(`export const en = { "worklist.untitled.task": "Task" };`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "de.ts"), []byte(`export const de = {};`), 0o600); err != nil {
		t.Fatal(err)
	}
	err = checkTitles(c, dir)
	if err == nil || !strings.Contains(err.Error(), "de.ts: worklist.untitled.task") {
		t.Errorf("got %v, want a refusal naming the locale and the missing title", err)
	}
}

// validKind is complete, so each refusal case below can only fail on the one
// change it makes.
const validKind = `
kinds:
  task:
    category: tasks
    destination: today
    owner: [named, nobody]
    personal: false
    done_in_place: false
`

func TestACatalogRefusesWhatNobodyDecided(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"a missing boolean":           {"    done_in_place: false\n", "", "done_in_place is missing"},
		"a missing category":          {"    category: tasks\n", "", "category"},
		"an unknown field":            {"    personal: false\n", "    personal: false\n    delivery: push\n", "delivery"},
		"a category outside the set":  {"category: tasks", "category: urgent", "category"},
		"a destination outside it":    {"destination: today", "destination: inbox", "destination"},
		"an owner outside the set":    {"owner: [named, nobody]", "owner: [named, everyone]", "owner"},
		"no owner answer":             {"owner: [named, nobody]", "owner: []", "at least one"},
		"an owner answer twice":       {"owner: [named, nobody]", "owner: [named, named]", "twice"},
		"a name that is no source":    {"  task:", "  Task-1:", "name"},
		"a name with an empty word":   {"  task:", "  task__due:", "name"},
		"a kind declared twice":       {"    done_in_place: false\n", "    done_in_place: false\n  task:\n    category: tasks\n", "already defined"},
		"a second document":           {"    done_in_place: false\n", "    done_in_place: false\n---\nkinds: {}\n", "one YAML document"},
		"a boolean that is no answer": {"personal: false", "personal: maybe", "into bool"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			doc := strings.Replace(validKind, c.from, c.to, 1)
			if doc == validKind {
				t.Fatalf("the case changed nothing: %q not found", c.from)
			}
			_, err := parseCatalog([]byte(doc))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want a refusal naming %q", err, c.want)
			}
		})
	}
}

func TestTheValidFixtureLoads(t *testing.T) {
	c, err := parseCatalog([]byte(validKind))
	if err != nil {
		t.Fatal(err)
	}
	if k := c["task"]; k.Category != "tasks" || *k.Personal || len(k.Owner) != 2 {
		t.Errorf("task parsed as %+v", k)
	}
}

func TestAnEmptyCatalogIsRefused(t *testing.T) {
	if _, err := parseCatalog([]byte("kinds: {}\n")); err == nil || !strings.Contains(err.Error(), "no kinds") {
		t.Errorf("got %v, want a refusal of an empty catalog", err)
	}
}
