// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations_test

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// TestEveryDeclaredRenumberPointsAtTheMigrationThatShips holds renumbered.txt
// to the shape the migrator trusts. Each pair's new version ships under its
// name, and its old version ships nothing. Its old digest is the content that
// ships today under the old version, so the bytes did not change. No old
// version appears on two lines.
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
	byVersion := make(map[string]dbmigrate.Migration, len(ns.Migrations))
	for _, m := range ns.Migrations {
		byVersion[m.Version] = m
	}
	olds := map[string]bool{}
	for _, p := range pairs {
		if olds[p.Old] {
			t.Errorf("%s %s is declared more than once", ns.Name, p.Old)
		}
		olds[p.Old] = true
		current, ok := byVersion[p.New]
		if !ok || current.Name != p.Name {
			t.Errorf("%s %s_%s → %s: the tree ships %q at %s, want %q", ns.Name, p.Old, p.Name, p.New, current.Name, p.New, p.Name)
			continue
		}
		if got, ok := byVersion[p.Old]; ok {
			t.Errorf("%s %s_%s: the old version still ships, as %q", ns.Name, p.Old, p.Name, got.Name)
		}
		asOld := current
		asOld.Version = p.Old
		if dbmigrate.Digest(asOld) != p.OldDigest {
			t.Errorf("%s %s_%s → %s: the declared old digest is not today's content under the old version, "+
				"so the SQL changed and the move cannot be declared", ns.Name, p.Old, p.Name, p.New)
		}
	}
	for _, p := range pairs {
		if olds[p.New] {
			t.Errorf("%s %s_%s → %s: the new version is itself declared renumbered, a chain", ns.Name, p.Old, p.Name, p.New)
		}
	}
}

// oldHeadManifest lists what commit e549efee shipped. It is the last head
// before the renumbers in renumbered.txt, so a database migrated there holds
// the old versions this upgrade has to reconcile.
const oldHeadManifest = "testdata/oldhead-e549efee.txt"

// TestTheOldHeadManifestResolves holds testdata/oldhead-e549efee.txt to the
// tree: each migration it lists ships today at that version, or is declared
// renumbered from it.
func TestTheOldHeadManifestResolves(t *testing.T) {
	text, err := os.ReadFile(oldHeadManifest)
	if err != nil {
		t.Fatalf("reading %s: %v", oldHeadManifest, err)
	}
	core, custom := namespaces(t)
	resolves := map[string]bool{}
	for _, ns := range []dbmigrate.Namespace{core, custom} {
		for _, m := range ns.Migrations {
			resolves[ns.Name+"/"+m.Version+"_"+m.Name] = true
		}
		for _, p := range ns.Renumbered {
			resolves[ns.Name+"/"+p.Old+"_"+p.Name] = true
		}
	}
	listed := 0
	for line := range strings.SplitSeq(string(text), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		listed++
		if !resolves[line] {
			t.Errorf("%s lists %s, which neither ships nor is declared renumbered", oldHeadManifest, line)
		}
	}
	if listed == 0 {
		t.Fatalf("%s lists no migration, so this test passes vacuously", oldHeadManifest)
	}
}
