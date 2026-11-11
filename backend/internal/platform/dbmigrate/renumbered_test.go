// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// renumberedNamespace ships one migration at 0020 that once shipped at 0010.
func renumberedNamespace() (Namespace, Migration) {
	current := Migration{Version: "0020", Name: "add_thing", UpSQL: "CREATE TABLE thing ()", DownSQL: "DROP TABLE thing"}
	return Namespace{
		Name:       "core",
		Migrations: []Migration{current},
		Renumbered: []Renumber{{Old: "0010", New: "0020", Name: "add_thing", OldDigest: *digestAs(current, "0010")}},
	}, current
}

// digestAs is the digest a database recorded when it applied m under version.
func digestAs(m Migration, version string) *string {
	m.Version = version
	return new(Digest(m))
}

func TestARenumberWhoseOldRowProvesTheContentMovesToTheNewVersion(t *testing.T) {
	t.Parallel()
	ns, current := renumberedNamespace()
	plan, err := planRenumbers(ns, map[string]LedgerRow{"0010": {Name: "add_thing", Digest: digestAs(current, "0010")}})
	if err != nil {
		t.Fatalf("a proved renumber was refused: %v", err)
	}
	if len(plan.steps) != 1 || plan.steps[0].dropOld {
		t.Fatalf("steps = %+v, want one move", plan.steps)
	}
	if _, stale := plan.ledger["0010"]; stale {
		t.Error("the old version is still in the planned ledger, so Up would judge it again")
	}
	if row := plan.ledger["0020"]; row.Digest == nil || *row.Digest != Digest(current) {
		t.Errorf("the moved row records %v, want today's digest so later runs compare equal", row.Digest)
	}
}

func TestARenumberWhoseNewRowAlsoProvesTheContentDropsTheOldRow(t *testing.T) {
	t.Parallel()
	ns, current := renumberedNamespace()
	plan, err := planRenumbers(ns, map[string]LedgerRow{
		"0010": {Name: "add_thing", Digest: digestAs(current, "0010")},
		"0020": {Name: "add_thing", Digest: digestAs(current, "0020")},
	})
	if err != nil {
		t.Fatalf("both rows proving one content was refused: %v", err)
	}
	if len(plan.steps) != 1 || !plan.steps[0].dropOld {
		t.Fatalf("steps = %+v, want one drop of the old row", plan.steps)
	}
}

// Each refusal is a ledger row the renumber cannot be proved from. Up must
// stop on it rather than move it, because a move would record DDL as run that
// this database ran in some other form.
func TestARenumberTheLedgerCannotProveIsRefused(t *testing.T) {
	t.Parallel()
	ns, current := renumberedNamespace()
	edited := current
	edited.UpSQL = "CREATE TABLE thing (id int)"
	for _, tc := range []struct {
		name    string
		ledger  map[string]LedgerRow
		expects string
	}{
		{
			name:    "the old row records edited content",
			ledger:  map[string]LedgerRow{"0010": {Name: "add_thing", Digest: digestAs(edited, "0010")}},
			expects: "different content",
		},
		{
			name:    "the old row records today's digest instead of the old version's",
			ledger:  map[string]LedgerRow{"0010": {Name: "add_thing", Digest: new(Digest(current))}},
			expects: "different content",
		},
		{
			name:    "the old row records no digest",
			ledger:  map[string]LedgerRow{"0010": {Name: "add_thing"}},
			expects: "no content digest",
		},
		{
			name:    "the old row names another migration",
			ledger:  map[string]LedgerRow{"0010": {Name: "add_other", Digest: digestAs(current, "0010")}},
			expects: `recorded the old version as "add_other"`,
		},
		{
			name: "the new row records other content",
			ledger: map[string]LedgerRow{
				"0010": {Name: "add_thing", Digest: digestAs(current, "0010")},
				"0020": {Name: "add_thing", Digest: digestAs(edited, "0020")},
			},
			expects: "also holds the new version",
		},
		{
			name: "the new row records no digest",
			ledger: map[string]LedgerRow{
				"0010": {Name: "add_thing", Digest: digestAs(current, "0010")},
				"0020": {Name: "add_thing"},
			},
			expects: "also holds the new version",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := planRenumbers(ns, tc.ledger)
			if err == nil {
				t.Fatal("the renumber was planned")
			}
			for _, want := range []string{tc.expects, "0010_add_thing", "0020", "Nothing was applied"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal %q does not mention %q", err, want)
				}
			}
		})
	}
}

// A declared pair the source contradicts is refused even with a proving
// ledger row. The declaration is what is wrong. Moving the row would record a
// migration the source does not ship.
func TestARenumberTheSourceContradictsIsRefused(t *testing.T) {
	t.Parallel()
	ns, current := renumberedNamespace()
	ledger := map[string]LedgerRow{"0010": {Name: "add_thing", Digest: digestAs(current, "0010")}}

	misnamed := ns
	misnamed.Renumbered = []Renumber{{Old: "0010", New: "0020", Name: "add_other"}}
	if _, err := planRenumbers(misnamed, ledger); err == nil || !strings.Contains(err.Error(), "does not carry that name") {
		t.Errorf("a pair whose new version ships another name: err = %v", err)
	}

	edited := ns
	edited.Migrations = []Migration{{Version: "0020", Name: "add_thing", UpSQL: "CREATE TABLE thing (id int)", DownSQL: "DROP TABLE thing"}}
	if _, err := planRenumbers(edited, ledger); err == nil || !strings.Contains(err.Error(), "no longer has the declared old content") {
		t.Errorf("a pair whose source changed since the declaration: err = %v", err)
	}

	stillShipped := ns
	stillShipped.Migrations = append([]Migration{{Version: "0010", Name: "add_thing", UpSQL: "SELECT 1", DownSQL: "SELECT 1"}}, ns.Migrations...)
	if _, err := planRenumbers(stillShipped, ledger); err == nil || !strings.Contains(err.Error(), "still ships") {
		t.Errorf("a pair whose old version still ships: err = %v", err)
	}
}

// A ledger without the old version is the common case, a fresh database
// among them. A declaration must leave it as it was.
func TestALedgerWithoutTheOldVersionIsLeftAlone(t *testing.T) {
	t.Parallel()
	ns, current := renumberedNamespace()
	ledger := map[string]LedgerRow{"0020": {Name: "add_thing", Digest: new(Digest(current))}}
	plan, err := planRenumbers(ns, ledger)
	if err != nil || len(plan.steps) != 0 || len(plan.ledger) != 1 {
		t.Fatalf("plan = %+v, err = %v; want no step and the ledger unchanged", plan, err)
	}
}

// Two old versions of one migration can both be in a ledger. The plan must
// end with one row at the new version and none at either old one.
func TestTwoOldVersionsOfOneMigrationEndAsOneRow(t *testing.T) {
	t.Parallel()
	ns, current := renumberedNamespace()
	ns.Renumbered = append(ns.Renumbered, Renumber{Old: "0005", New: "0020", Name: "add_thing", OldDigest: *digestAs(current, "0005")})
	plan, err := planRenumbers(ns, map[string]LedgerRow{
		"0005": {Name: "add_thing", Digest: digestAs(current, "0005")},
		"0010": {Name: "add_thing", Digest: digestAs(current, "0010")},
	})
	if err != nil {
		t.Fatalf("two proved old rows were refused: %v", err)
	}
	if len(plan.ledger) != 1 || len(plan.steps) != 2 || plan.steps[0].dropOld || !plan.steps[1].dropOld {
		t.Fatalf("ledger = %v, steps = %+v; want 0005 moved, then 0010 dropped", plan.ledger, plan.steps)
	}
}

// A database that applied a moved migration in a form an equivalence entry
// names for the new version is moved, not stopped. Read from the shipped
// renumbered.txt and equivalentContent, so every such pair is asked.
func TestARenumberFromEquivalentContentMovesTheRecordedRow(t *testing.T) {
	t.Parallel()
	text, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "renumbered.txt"))
	if err != nil {
		t.Fatalf("reading renumbered.txt: %v", err)
	}
	declared, err := ParseRenumbers(string(text))
	if err != nil {
		t.Fatalf("parsing renumbered.txt: %v", err)
	}
	shipped := map[string]Migration{}
	for _, m := range loadNamespaceMigrations(t, "core") {
		shipped[m.Version] = m
	}
	asked := 0
	for _, pair := range declared["core"] {
		for _, eq := range equivalentContent["core"][pair.New] {
			asked++
			ns := Namespace{Name: "core", Migrations: []Migration{shipped[pair.New]}, Renumbered: []Renumber{pair}}
			plan, err := planRenumbers(ns, map[string]LedgerRow{pair.Old: {Name: pair.Name, Digest: new(eq.applied)}})
			if err != nil {
				t.Errorf("%s -> %s: a database holding the equivalent content was refused: %v", pair.Old, pair.New, err)
				continue
			}
			if len(plan.steps) != 1 || plan.steps[0].recorded != eq.applied {
				t.Errorf("%s -> %s: steps = %+v, want one move matching the recorded digest", pair.Old, pair.New, plan.steps)
			}
			stranger := "bb" + strings.Repeat("0", 62)
			if _, err := planRenumbers(ns, map[string]LedgerRow{pair.Old: {Name: pair.Name, Digest: &stranger}}); err == nil {
				t.Errorf("%s -> %s: content no entry names was moved", pair.Old, pair.New)
			}
		}
	}
	if asked == 0 {
		t.Fatal("no renumbered pair has an equivalence entry, so nothing here was asked; remove this test with the last such entry")
	}
}
