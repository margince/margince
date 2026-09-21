// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations

// A migration that adds a constraint NOT VALID and validates it in the SAME
// file has bought nothing, and the comment above it says otherwise.
//
// The claim being made is true of Postgres and false of this runner. Splitting
// the add from the validation is worth doing because ADD CONSTRAINT ... CHECK
// scans every row while holding ACCESS EXCLUSIVE, while VALIDATE CONSTRAINT
// takes only SHARE UPDATE EXCLUSIVE and lets writers through. That only pays
// when the two run in DIFFERENT transactions. dbmigrate.Up applies a whole
// migration file inside ONE, so the ACCESS EXCLUSIVE the ALTER took is held
// until commit and the VALIDATE scan runs entirely underneath it. Writers are
// blocked for exactly as long, plus a second pass over the table.
//
// So the pattern is not merely neutral here — it is a slower spelling of ADD
// CONSTRAINT that reads like a lock optimisation. That is the damage: it reads
// as a pattern to copy, and it HAS been copied. Twenty-three files carried it
// when this gate landed, one of them saying in its own comment that it is "the
// shape ... asks a later migration to copy".
//
// Split across two migrations it is real, and this gate says nothing about
// that: 1787968162 adds a currency CHECK NOT VALID and 1787968163 validates it,
// two files, two transactions, and the writers between them go through. That is
// what the pattern is for, and it is the shape a contact reaching for it should
// find.

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// notValidBaseline is where this obligation starts in each namespace.
//
// Every file already carrying the pattern has SHIPPED, and a migration that has
// run somewhere is not editable — rewriting one changes what a database that
// already applied it is assumed to contain. So the backlog is pinned out rather
// than fixed, and the pin sits one above the highest version each namespace
// holds today.
//
// LIKE lockTimeoutBaseline next door, this arms itself and does not need
// anybody to remember it: a new core migration is named for the unix second it
// was written and a custom one for its timestamp, so both sort above these and
// the very next migration in either namespace is read. The examined/skipped
// split is logged for the same reason it is there — a pass over an empty set
// and a pass over real files must not read alike.
// gatekit:fixture the oldest version this gate binds in each namespace — data,
// not a cost: a namespace's entry names where the rule starts applying.
var notValidBaseline = map[string]string{
	"core":   "1788680101",
	"custom": "20260825120001",
}

var (
	// addsNotValid captures the constraint an ALTER adds without validating.
	// The name is what makes the pairing decidable: a file may legitimately add
	// one constraint NOT VALID and validate a DIFFERENT one that an earlier
	// migration left pending, and that is the split pattern working.
	addsNotValid = regexp.MustCompile(`(?is)\bADD\s+CONSTRAINT\s+([\w".]+)\b[^;]*?\bNOT\s+VALID\s*;`)
	// validatesConstraint captures the constraint a VALIDATE names.
	validatesConstraint = regexp.MustCompile(`(?is)\bVALIDATE\s+CONSTRAINT\s+([\w".]+)`)
)

// TestNoMigrationValidatesAConstraintItAddedInTheSameFile reports every file
// that adds a constraint NOT VALID and then validates it before the transaction
// that added it has committed.
func TestNoMigrationValidatesAConstraintItAddedInTheSameFile(t *testing.T) {
	// Read off the embedded tree rather than listed, so a namespace added later
	// is covered without anybody remembering this file.
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatalf("reading the embedded migration namespaces: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		namespace := entry.Name()
		t.Run(namespace, func(t *testing.T) {
			// A namespace with no baseline would take the zero value "" and arm
			// its whole backlog at once, which reads as this gate breaking rather
			// than as a decision somebody owes.
			if _, set := notValidBaseline[namespace]; !set {
				t.Fatalf("namespace %q has no notValidBaseline: pick the version this "+
					"obligation starts at for it, in the same change that adds the namespace", namespace)
			}
			dir, subErr := fs.Sub(files, namespace)
			if subErr != nil {
				t.Fatalf("reading the %s namespace: %v", namespace, subErr)
			}
			checkNotValidSplits(t, namespace, dir)
		})
	}
}

func checkNotValidSplits(t *testing.T, namespace string, dir fs.FS) {
	t.Helper()
	var examined, skipped int
	err := fs.WalkDir(dir, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".sql") {
			return err
		}
		body, readErr := fs.ReadFile(dir, path)
		if readErr != nil {
			return readErr
		}
		if version(path) < notValidBaseline[namespace] {
			skipped++
			return nil
		}
		examined++
		for _, name := range validatedInTheSameFile(string(body)) {
			t.Errorf("%s/%s adds %s NOT VALID and validates it in the same file, which buys nothing: "+
				"dbmigrate.Up runs the whole file in ONE transaction, so the ACCESS EXCLUSIVE the ALTER "+
				"took is still held when VALIDATE runs and writers are blocked for the whole of both.\n"+
				"Add it plainly (ADD CONSTRAINT ... CHECK), or put the VALIDATE in its own migration — "+
				"two files are two transactions, and that is the only shape where the split pays.",
				namespace, path, name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the migration files: %v", err)
	}
	// A pass over nothing and a pass over real files must not read alike: this
	// gate is dormant by design until the next migration lands, and the numbers
	// are what say which of the two produced the pass.
	if examined == 0 && skipped == 0 {
		t.Errorf("%s: no .sql files at all — the embedded namespace is empty, so this gate "+
			"read nothing and passed", namespace)
	}
	t.Logf("%s: examined %d file(s), %d below the %s pin",
		namespace, examined, skipped, notValidBaseline[namespace])
}

// validatedInTheSameFile names every constraint this file both adds NOT VALID
// and validates.
//
// Comments and string literals are stripped first (executableSQL): a file that
// EXPLAINS the pattern in prose — as several do, and as the doc comment on
// dbmigrate.Up now does — is not running it.
//
// Matching is by NAME rather than by mere presence of both phrases, because
// validating a constraint an EARLIER migration left pending is the pattern
// working exactly as intended, and it appears in the same file shape.
func validatedInTheSameFile(sql string) []string {
	statements := executableSQL(sql)
	// POSITION, not merely presence. A file may validate an earlier `foo`, drop
	// it, and add a replacement `foo` NOT VALID — which leaves the replacement
	// pending, exactly as intended. Reading the two sets independently would
	// pair that VALIDATE with the ADD that came after it and block a correct
	// migration, so an ADD counts only against a VALIDATE that FOLLOWS it.
	addedAt := map[string]int{}
	for _, m := range addsNotValid.FindAllStringSubmatchIndex(statements, -1) {
		name := constraintName(statements[m[2]:m[3]])
		if _, seen := addedAt[name]; !seen {
			addedAt[name] = m[0]
		}
	}
	if len(addedAt) == 0 {
		return nil
	}
	var found []string
	reported := map[string]bool{}
	for _, m := range validatesConstraint.FindAllStringSubmatchIndex(statements, -1) {
		name := constraintName(statements[m[2]:m[3]])
		at, added := addedAt[name]
		if !added || m[0] < at || reported[name] {
			continue
		}
		reported[name] = true
		found = append(found, name)
	}
	return found
}

// constraintName normalizes an identifier the way Postgres reads one: an
// unquoted name folds to lower case, a quoted one keeps exactly the case it was
// written in. Folding both would make `"Foo"` and `foo` — which are two
// different constraints to the server — read here as one, and block a migration
// that adds the first while validating the second.
func constraintName(raw string) string {
	if strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) && len(raw) > 1 {
		return strings.Trim(raw, `"`)
	}
	return strings.ToLower(raw)
}
