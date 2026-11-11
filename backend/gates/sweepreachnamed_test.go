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

// TestOnlyTheBootSealsTheDeclaredTables holds the two ways the set is sealed to
// their one caller each. corereach.Install always carries the core modules'
// declarations, so a sealed production set cannot lack them; the empty test
// seal belongs to the test database alone.
func TestOnlyTheBootSealsTheDeclaredTables(t *testing.T) {
	t.Parallel()
	root := gatekit.ModuleRoot(t)
	want := map[string]string{
		"sweepreach.Install(":                "internal/compose/corereach/corereach.go",
		"sweepreach.InstallForTestDatabase(": "internal/platform/testdb/",
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
					t.Errorf("%s calls %s; only %s may seal the declared tables", rel, call, owner)
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
