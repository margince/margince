// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestARenameChainLandsOnTheNameTheDatabaseHasToday(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{
		"0001_first.up.sql":    "ALTER TABLE person RENAME TO contact;",
		"0002_second.up.sql":   "ALTER TABLE contact RENAME TO party;\nALTER INDEX person_pkey RENAME TO party_pkey;",
		"0002_second.down.sql": "ALTER TABLE party RENAME TO lost;",
	})
	if got := CurrentNames(dir)["person"]; got != "party" {
		t.Errorf("person ended up as %q, want party: a name renamed twice keeps one entry for where it landed", got)
	}
	if got := WithCurrentNames(dir, "SELECT 1 FROM person JOIN person_pkey_x ON true"); got != "SELECT 1 FROM party JOIN person_pkey_x ON true" {
		t.Errorf("the rewrite read %q: it renames whole words only, and never a down migration's names", got)
	}
}

func TestADirectoryWithNoMigrationsRenamesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if renames := CurrentNames(dir); renames != nil {
		t.Errorf("an empty directory gave renames %v", renames)
	}
	if got := WithCurrentNames(dir, "SELECT 1 FROM person"); got != "SELECT 1 FROM person" {
		t.Errorf("with no renames the SQL changed to %q", got)
	}
}

func TestPackageStringConstantsReadsOnlyTheProductionFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{
		"words.go":      "package words\n\nconst Greeting = \"hello\"\n",
		"words_test.go": "package words\n\nconst Fixture = \"planted\"\n",
	})
	got := PackageStringConstants(t, dir)
	if got["Greeting"] != "hello" {
		t.Errorf("the package constant read as %q, want hello", got["Greeting"])
	}
	if _, read := got["Fixture"]; read {
		t.Error("a test file's constant was read as the package's own")
	}
}
