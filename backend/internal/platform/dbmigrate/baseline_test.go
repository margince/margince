// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

import (
	"strings"
	"testing"
)

func TestParseBaselineManifestReadsTheReleaseAndEachVersion(t *testing.T) {
	release, replaces, err := ParseBaselineManifest("# header\nrelease v1.2.3\n\n0001 first aa\n0002 second bb\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if release != "v1.2.3" {
		t.Errorf("release = %q, want v1.2.3", release)
	}
	want := []LedgerEntry{{"0001", "first", "aa"}, {"0002", "second", "bb"}}
	if len(replaces) != len(want) || replaces[0] != want[0] || replaces[1] != want[1] {
		t.Errorf("replaces = %+v, want %+v", replaces, want)
	}
}

func TestParseBaselineManifestRefusesAMalformedOne(t *testing.T) {
	for name, text := range map[string]string{
		"no release":     "0001 first aa\n",
		"no versions":    "release v1\n",
		"two releases":   "release v1\nrelease v2\n0001 first aa\n",
		"a short line":   "release v1\n0001 first\n",
		"a long line":    "release v1\n0001 first aa extra\n",
		"nothing at all": "",
	} {
		if _, _, err := ParseBaselineManifest(text); err == nil {
			t.Errorf("%s: parsed without an error", name)
		}
	}
}

// baselineNamespace is two replaced migrations, the baseline above them, and
// one migration after it: the shape core has.
func baselineNamespace() Namespace {
	first := Migration{Version: "0001", Name: "first", UpSQL: "CREATE TABLE a ();", DownSQL: "DROP TABLE a;"}
	second := Migration{Version: "0002", Name: "second", UpSQL: "CREATE TABLE b ();", DownSQL: "DROP TABLE b;"}
	later := Migration{Version: "0004", Name: "later", UpSQL: "CREATE TABLE c ();", DownSQL: "DROP TABLE c;"}
	return Namespace{
		Name:       "core",
		Migrations: []Migration{first, second, later},
		Baseline: &Baseline{
			Migration: Migration{Version: "0003", Name: "baseline", UpSQL: "CREATE TABLE a (); CREATE TABLE b ();"},
			Release:   "v1",
			Replaces: []LedgerEntry{
				{Version: first.Version, Name: first.Name, Digest: Digest(first)},
				{Version: second.Version, Name: second.Name, Digest: Digest(second)},
			},
		},
	}
}

func TestCheckBaselineAcceptsAManifestThatMatchesItsFiles(t *testing.T) {
	if err := baselineNamespace().CheckBaseline(); err != nil {
		t.Fatalf("a matching baseline was refused: %v", err)
	}
	if err := (Namespace{Name: "custom"}).CheckBaseline(); err != nil {
		t.Fatalf("a namespace with no baseline was refused: %v", err)
	}
}

// A replaced file may go once every installation is past the baseline. A
// manifest entry with no file beside it is that later shape.
func TestCheckBaselineAcceptsAReplacedVersionWhoseFileIsGone(t *testing.T) {
	ns := baselineNamespace()
	ns.Migrations = ns.Migrations[2:]
	if err := ns.CheckBaseline(); err != nil {
		t.Fatalf("a baseline whose replaced files are gone was refused: %v", err)
	}
}

func TestCheckBaselineRefusesAManifestThatDisagreesWithItsFiles(t *testing.T) {
	for _, tc := range []struct {
		name    string
		corrupt func(*Namespace)
		expects string
	}{
		{
			name:    "a file below the baseline is not listed",
			corrupt: func(ns *Namespace) { ns.Baseline.Replaces = ns.Baseline.Replaces[:1] },
			expects: "does not list it",
		},
		{
			name:    "a listed file was renamed",
			corrupt: func(ns *Namespace) { ns.Migrations[0].Name = "renamed" },
			expects: "different name or content",
		},
		{
			name:    "a listed file was edited",
			corrupt: func(ns *Namespace) { ns.Migrations[1].UpSQL += " -- edited" },
			expects: "different name or content",
		},
		{
			name:    "a listed version sorts above the baseline",
			corrupt: func(ns *Namespace) { ns.Baseline.Replaces[1].Version = "0009" },
			expects: "does not sort below it",
		},
		{
			name:    "a version is listed twice",
			corrupt: func(ns *Namespace) { ns.Baseline.Replaces[1] = ns.Baseline.Replaces[0] },
			expects: "twice",
		},
		{
			name:    "a migration carries the baseline's version",
			corrupt: func(ns *Namespace) { ns.Migrations[2].Version = "0003" },
			expects: "both a migration and the baseline",
		},
		{
			name: "the baseline asks to run unwrapped",
			corrupt: func(ns *Namespace) {
				ns.Baseline.Migration.UpSQL = NoTransactionMarker + "\n" + ns.Baseline.Migration.UpSQL
			},
			expects: "cannot ask to run unwrapped",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ns := baselineNamespace()
			tc.corrupt(&ns)
			err := ns.CheckBaseline()
			if err == nil || !strings.Contains(err.Error(), tc.expects) {
				t.Fatalf("err = %v, want one containing %q", err, tc.expects)
			}
		})
	}
}

// The ledger at head holds each shipped migration and the baseline. Once a
// replaced file is gone, its row still comes from the manifest.
func TestLedgerAtHeadKeepsAReplacedVersionWhoseFileIsGone(t *testing.T) {
	ns := baselineNamespace()
	versions := func(entries []LedgerEntry) string {
		var v []string
		for _, e := range entries {
			v = append(v, e.Version)
		}
		return strings.Join(v, " ")
	}
	if got := versions(ns.LedgerAtHead()); got != "0001 0002 0004 0003" {
		t.Errorf("with every file: %s", got)
	}
	ns.Migrations = ns.Migrations[2:]
	if got := versions(ns.LedgerAtHead()); got != "0004 0003 0001 0002" {
		t.Errorf("with the replaced files gone: %s", got)
	}
}

// ledgerOf is the ledger rows of entries, as Up reads them back.
func ledgerOf(entries ...LedgerEntry) map[string]LedgerRow {
	ledger := map[string]LedgerRow{}
	for _, e := range entries {
		digest := e.Digest
		ledger[e.Version] = LedgerRow{Name: e.Name, Digest: &digest}
	}
	return ledger
}

// Once the replaced files are gone, only the manifest says what their rows
// must hold. A row that disagrees is refused before anything is written.
func TestPlanBaselineRefusesAReplacedRowThatDisagreesWithTheManifest(t *testing.T) {
	ns := baselineNamespace()
	ns.Migrations = ns.Migrations[2:]
	b := ns.Baseline
	own := LedgerEntry{Version: b.Migration.Version, Name: b.Migration.Name, Digest: Digest(b.Migration)}
	edited := b.Replaces[1]
	edited.Digest = "another"

	for name, ledger := range map[string]map[string]LedgerRow{
		"at the release":        ledgerOf(b.Replaces[0], edited),
		"the baseline recorded": ledgerOf(b.Replaces[0], edited, own),
	} {
		if _, err := planBaseline(ns, ledger); err == nil || !strings.Contains(err.Error(), "other content than the baseline") {
			t.Errorf("%s: err = %v, want a refusal of %s", name, err, edited.Version)
		}
	}
	if plan, err := planBaseline(ns, ledgerOf(b.Replaces...)); err != nil || plan.apply || len(plan.rows) != 1 {
		t.Errorf("a matching ledger at the release: plan %+v, err %v; want the baseline's row alone", plan, err)
	}
}

// A partial history is told which release to reach first. Before the first
// release there is none, so it is told to build the database again.
func TestPlanBaselineTellsAPartialHistoryWhatToDo(t *testing.T) {
	for release, expects := range map[string]string{
		"v1":             "Upgrade it with release v1 first",
		NoEarlierRelease: "build this database again",
	} {
		ns := baselineNamespace()
		ns.Baseline.Release = release
		_, err := planBaseline(ns, ledgerOf(ns.Baseline.Replaces[0]))
		if err == nil || !strings.Contains(err.Error(), expects) {
			t.Errorf("release %q: err = %v, want one containing %q", release, err, expects)
		}
	}
}
