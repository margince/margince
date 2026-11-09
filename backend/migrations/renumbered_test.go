// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// TestEveryDeclaredRenumberPointsAtTheMigrationThatShips holds renumbered.txt
// to the shape the migrator trusts. Each pair's new version ships under its
// name, and its old version ships nothing. No old version appears on two lines.
// No new version is another pair's old one: a chain would leave a row at a
// version the migrator never revisits.
func TestEveryDeclaredRenumberPointsAtTheMigrationThatShips(t *testing.T) {
	declared, err := migrations.Renumbered()
	if err != nil {
		t.Fatalf("reading renumbered.txt: %v", err)
	}
	core, custom := namespaces(t)
	shipped := map[string]dbmigrate.Namespace{core.Name: core, custom.Name: custom}

	if len(declared[core.Name]) == 0 {
		t.Fatal("renumbered.txt declares no core renumber, so every check below passes vacuously")
	}
	for name, pairs := range declared {
		ns, ok := shipped[name]
		if !ok {
			t.Errorf("renumbered.txt names namespace %q, which no loader attaches", name)
			continue
		}
		if len(ns.Renumbered) != len(pairs) {
			t.Errorf("%s: the loader attached %d pairs where renumbered.txt declares %d", name, len(ns.Renumbered), len(pairs))
		}
		holdsPairs(t, ns, pairs)
	}
}

func holdsPairs(t *testing.T, ns dbmigrate.Namespace, pairs []dbmigrate.Renumber) {
	t.Helper()
	byVersion := make(map[string]string, len(ns.Migrations))
	for _, m := range ns.Migrations {
		byVersion[m.Version] = m.Name
	}
	olds := map[string]bool{}
	for _, p := range pairs {
		if olds[p.Old] {
			t.Errorf("%s %s is declared more than once", ns.Name, p.Old)
		}
		olds[p.Old] = true
		if got, ok := byVersion[p.New]; !ok || got != p.Name {
			t.Errorf("%s %s_%s → %s: the tree ships %q at %s, want %q", ns.Name, p.Old, p.Name, p.New, got, p.New, p.Name)
		}
		if got, ok := byVersion[p.Old]; ok {
			t.Errorf("%s %s_%s: the old version still ships, as %q", ns.Name, p.Old, p.Name, got)
		}
	}
	for _, p := range pairs {
		if olds[p.New] {
			t.Errorf("%s %s_%s → %s: the new version is itself declared renumbered, a chain", ns.Name, p.Old, p.Name, p.New)
		}
	}
}
