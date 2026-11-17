// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/corereach"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// TestNoCoreCodeWritesADeclaredColumnByName fails when code outside a table's
// owner writes a column the owner declared for the merge or the erasure to
// reach (sweepreach). That write belongs to the declaration, so the owner can
// leave core without the merge or the erasure losing the table.
func TestNoCoreCodeWritesADeclaredColumnByName(t *testing.T) {
	t.Parallel()
	declared := map[string][]string{}
	for _, o := range corereach.Tables() {
		owner := "internal/modules/" + o.Owner
		if tableOwners[o.Table.Name] != owner {
			t.Errorf("%s declares reach into %s, which tableOwners gives to %q", owner, o.Table.Name, tableOwners[o.Table.Name])
		}
		for _, l := range o.Table.Links {
			declared[o.Table.Name] = append(declared[o.Table.Name], l.Column)
		}
		for _, c := range o.Table.Erase {
			declared[o.Table.Name] = append(declared[o.Table.Name], c.Column)
		}
	}
	if len(declared) == 0 {
		t.Fatal("no core module declares reach; the census is reading nothing")
	}
	for _, s := range sqlStatementsUnder(t, gatekit.ModuleRoot(t)) {
		for _, table := range sqlWriteTargets(s.sql) {
			columns, isDeclared := declared[table]
			if !isDeclared || strings.HasPrefix(s.where, tableOwners[table]+"/") {
				continue
			}
			for _, column := range columns {
				if assignsColumn(s.sql, column) {
					t.Errorf("%s writes %s.%s by name, which its owner declares for the merge "+
						"and erasure to reach; drop the statement and let sweepreach run it", s.where, table, column)
				}
			}
		}
	}
}

// TestOnlyTheBootBuildsTheDeclaredTables holds the set a merge or erasure
// reaches to its one builder and its one hand-over. corereach.Set always
// carries the core modules' declarations, and only the composition's privacy
// registry carries a set to a door, so no production door reaches fewer.
func TestOnlyTheBootBuildsTheDeclaredTables(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	want := map[string]string{
		"sweepreach.NewSet(": "internal/compose/corereach/corereach.go",
		".WithReach(":        "internal/compose/extensionprivacy/",
	}
	found := map[string]int{}
	for _, dir := range []string{"internal", "cmd"} {
		for _, path := range goSourceFiles(t, filepath.Join(root, dir)) {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			body, err := gatekit.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			for call, owner := range want {
				if !strings.Contains(string(body), call) {
					continue
				}
				found[call]++
				if !strings.HasPrefix(filepath.ToSlash(rel), owner) {
					t.Errorf("%s calls %s; only %s may build or hand over the declared tables", rel, call, owner)
				}
			}
		}
	}
	for call := range want {
		if found[call] == 0 {
			t.Errorf("no file calls %s; the census is reading the wrong tree", call)
		}
	}
}

// TestEveryDeclaredTableHasARowID fails when a core table declared for the
// merge has no uuid id: the merge audits each row it moves by that id.
func TestEveryDeclaredTableHasARowID(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatal(err)
	}
	tables := corereach.Tables()
	if len(tables) == 0 {
		t.Fatal("no core module declares reach; the census is reading nothing")
	}
	for _, o := range tables {
		if !strings.Contains(string(raw), "\npublic."+o.Table.Name+".id uuid NOT NULL ") {
			t.Errorf("%s declares %s for the merge, which has no NOT NULL uuid id to audit a moved row by", o.Owner, o.Table.Name)
		}
	}
}
