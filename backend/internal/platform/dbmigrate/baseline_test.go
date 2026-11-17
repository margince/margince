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
	want := []Replaced{{"0001", "first", "aa"}, {"0002", "second", "bb"}}
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
			Replaces: []Replaced{
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
