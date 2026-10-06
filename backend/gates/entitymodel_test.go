// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The entity model pages say what the schema says.
//
// docs/reference/entity-model/ is what an engineer reads to learn the records
// this product keeps. A page like that is written once and then quietly stops
// being true — a column is added, a reference changes direction, a table is
// dropped — and its readers have no way to tell, because a stale page renders
// exactly like a current one. So it is not written. It is rendered, here, from
// three sources that a change to the model has to touch anyway:
//
//   - migrations/testdata/head_catalog.txt — the committed schema. A migration
//     updates it in the same commit or TestMigrationsBuildTheCommittedSchema
//     fails, so it cannot lag the database.
//   - api/crm.yaml — the contract, for what a field is FOR.
//   - tableOwners — which module owns each table's writes.
//
// Nothing else reaches the page, and that answers the question this gate is
// really for: whether ordinary Go changes should regenerate it. They cannot
// change it. Only a migration, the contract or the ownership map can, and each
// of those fails HERE on the same run that changed it.
//
// The comparison is both directions. A page the render no longer produces is as
// wrong as one it produces differently: a module that loses its last table
// leaves a file behind describing tables that have moved, and a reader who
// finds it has no reason to doubt it.

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const entityModelDir = "../docs/reference/entity-model"

var updateEntityModel = flag.Bool("update-entity-model", false,
	"rewrite docs/reference/entity-model/ from the committed schema, the contract and the ownership map")

// regenerate is the command the failure messages print.
const regenerate = "cd backend && go test ./gates -run EntityModel -update-entity-model"

const entityModelLede = `Every table this product keeps, what each column is, and how the records point
at each other.

Rendered from the schema the migrations build (` + "`backend/migrations/testdata/head_catalog.txt`" + `),
the field descriptions in the API contract (` + "`backend/api/crm.yaml`" + `), and the
module that owns each table's writes. Changing the model means changing one of
those three, and the gate that renders this fails on the same run, so
regenerate it with the change that moved it:

    cd backend && go test ./gates -run EntityModel -update-entity-model

A column whose last cell is empty has no description anywhere in the tree: the
catalog knows its type and nothing has said what it is for. Give it one by
adding a ` + "`description:`" + ` to the matching field in ` + "`backend/api/crm.yaml`" + `, and
the next regeneration picks it up. A foreign-key column names its target; what
happens when the parent is deleted is in the table's **Points at** list.

For why the schema is shaped this way rather than what it holds, read
[explanation/architecture.md](../../explanation/architecture.md) and
[explanation/write-backbone.md](../../explanation/write-backbone.md). For which
module to put a change in, read [modules.md](../modules.md).
`

func TestEntityModelPagesMatchTheSchema(t *testing.T) {
	t.Parallel()
	schema := parseHeadCatalog(t)
	rendered := entityModelPages(schema, tableOwners, contractDescriptions(t, schema))

	if *updateEntityModel {
		writeEntityModel(t, rendered)
		return
	}
	onDisk := entityModelOnDisk(t)
	for name, want := range rendered {
		got, present := onDisk[name]
		if !present {
			t.Errorf("%s/%s is missing — the model describes an area that has no page.\n"+
				"Regenerate with:\n    %s", entityModelDir, name, regenerate)
			continue
		}
		if got != want {
			t.Errorf("%s/%s no longer describes the schema: %s.\nRegenerate with:\n    %s",
				entityModelDir, name, firstDifference(got, want), regenerate)
		}
	}
	for name := range onDisk {
		if _, wanted := rendered[name]; !wanted {
			t.Errorf("%s/%s describes an area that no longer owns a table. A page nothing "+
				"renders is a page nothing corrects.\nRegenerate with:\n    %s",
				entityModelDir, name, regenerate)
		}
	}
}

// TestEntityModelDrawsOnEveryColumnItsSourcesCarry is the floor under the gate
// above.
//
// The parity check compares the pages to the render, and both would agree
// perfectly about a schema half of which never arrived: a parser that silently
// dropped a class of line renders short pages, this test rewrites them, and the
// diff a reviewer reads is the evidence disappearing. So the render is measured
// against the sources rather than against itself — every table the ownership map
// names must appear on a page, and every column of every table must be printed.
func TestEntityModelDrawsOnEveryColumnItsSourcesCarry(t *testing.T) {
	t.Parallel()
	schema := parseHeadCatalog(t)
	pages := entityModelPages(schema, tableOwners, contractDescriptions(t, schema))

	printed, columns := 0, 0
	for _, name := range schema.tableNames() {
		page, ok := pages[areaOf(tableOwners[name])+".md"]
		if !ok {
			t.Errorf("table %q lands on no page", name)
			continue
		}
		if !strings.Contains(page, "\n## "+name+"\n") {
			t.Errorf("table %q has a page but no section on it", name)
			continue
		}
		printed++
		for _, column := range schema.tables[name].columns {
			columns++
			if !strings.Contains(page, "| `"+column.name+"` | "+cell(typeCell(column))+" |") {
				t.Errorf("%s.%s is in the catalog and on no page", name, column.name)
			}
		}
	}
	if printed < catalogTableFloor || columns < catalogColumnFloor {
		t.Fatalf("the model printed %d table(s) and %d column(s), below the floor of %d and %d — "+
			"the render has stopped covering the schema", printed, columns, catalogTableFloor, catalogColumnFloor)
	}
}

func writeEntityModel(t *testing.T, pages map[string]string) {
	t.Helper()
	if err := os.MkdirAll(entityModelDir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", entityModelDir, err)
	}
	for name := range entityModelOnDisk(t) {
		if _, keep := pages[name]; keep {
			continue
		}
		if err := os.Remove(filepath.Join(entityModelDir, name)); err != nil {
			t.Fatalf("removing the stale page %s: %v", name, err)
		}
	}
	for name, body := range pages {
		if err := os.WriteFile(filepath.Join(entityModelDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	t.Logf("rewrote %d page(s) under %s — commit them with the change that moved the model",
		len(pages), entityModelDir)
}

// entityModelOnDisk reads the pages that are there now. A missing directory is
// an empty set rather than a failure, so the first run can create it.
func entityModelOnDisk(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(entityModelDir)
	if os.IsNotExist(err) {
		return map[string]string{}
	}
	if err != nil {
		t.Fatalf("reading %s: %v", entityModelDir, err)
	}
	pages := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(entityModelDir, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		pages[entry.Name()] = string(body)
	}
	return pages
}

// firstDifference names the first line that differs, because the whole diff of a
// generated page is the page.
func firstDifference(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range max(len(gotLines), len(wantLines)) {
		switch {
		case i >= len(gotLines):
			return "the page stops at line " + strconv.Itoa(i) + ", where the schema has " + quote(wantLines[i])
		case i >= len(wantLines):
			return "line " + strconv.Itoa(i+1) + " is " + quote(gotLines[i]) + ", which the schema no longer has"
		case gotLines[i] != wantLines[i]:
			return "line " + strconv.Itoa(i+1) + " says " + quote(gotLines[i]) + ", the schema says " + quote(wantLines[i])
		}
	}
	return "the pages differ only in trailing whitespace"
}

func quote(line string) string { return `"` + truncate(strings.TrimSpace(line), 120) + `"` }
