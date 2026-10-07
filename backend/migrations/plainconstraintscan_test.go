// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations

// A constraint added to a table this file did not create is added NOT VALID.
//
// `ADD CONSTRAINT … CHECK` and `ADD CONSTRAINT … FOREIGN KEY` scan every existing
// row WHILE HOLDING ACCESS EXCLUSIVE. On a mature table that scan is the stall, and
// `lock_timeout` does not reach it: that bounds how long the statement waits to
// acquire the lock, never how long it holds one. NOT VALID records the constraint
// without the scan and takes the lock only long enough to do it; a VALIDATE in a
// LATER file then checks the existing rows under SHARE UPDATE EXCLUSIVE, which
// readers and writers pass.
//
// Later, and that is the whole of it — notvalidsplit_test.go holds the other half,
// that the two do not sit in one file. This runner applies a file in one
// transaction, so NOT VALID followed by VALIDATE in the same file holds the same
// ACCESS EXCLUSIVE for longer than the plain spelling would.
//
// A table the file CREATES is exempt, and not as a convenience: a table that does
// not exist yet has no rows to scan and nobody waiting behind it, so demanding NOT
// VALID there would teach the next reader that the pattern is a ritual rather than a
// scan budget.

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// plainConstraintBaseline is where this obligation starts in each namespace.
//
// Above main's highest version, which spares a backlog the same way
// lockTimeoutBaseline and notValidBaseline do: 154 shipped files add a plain
// constraint to a table they did not create, and a shipped migration cannot be
// edited — rewriting one changes what a FRESH installation gets while every deployed
// database keeps what it already ran, and the two then disagree about the schema
// they are supposed to share.
//
// So this reads nothing today and arms on the next migration written. The count
// below is what keeps that honest rather than quietly permanent.
//
// gatekit:fixture the oldest version this gate binds in each namespace — data, not a
// cost: a namespace's entry names where the rule starts applying.
var plainConstraintBaseline = map[string]string{
	"core":   "1790990000",
	"custom": "20261002000000",
}

// alteredTable captures the table an ALTER names. `ONLY` is accepted because
// Postgres accepts it and it alters the same table — reading past it would leave
// every `ALTER TABLE ONLY` uninspected, which is under-recognition rather than
// permission.
var alteredTable = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+(?:ONLY\s+)?(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?([\w".]+)`)

// scanningConstraint captures the HEAD of one ADD CONSTRAINT whose kind scans the
// table. Where the action ENDS is found by scanning, not by this pattern: a CHECK
// body nests parentheses — `CHECK ((x > 0) AND (y > 0))` — and no regular expression
// closes them, so a pattern that tried would stop early and read a compliant
// constraint's NOT VALID as absent. That failure direction is the worse one: it
// blocks a correct migration rather than letting a wrong one through.
//
// UNIQUE and PRIMARY KEY are absent on purpose: they build an index rather than
// validate a predicate, and NOT VALID is not a thing Postgres accepts for them.
var scanningConstraint = regexp.MustCompile(
	`(?is)\bADD\s+CONSTRAINT\s+([\w".]+)[^,;]*?\b(CHECK|FOREIGN\s+KEY)\b`,
)

// notValidTail reports whether the matched statement ends NOT VALID.
var notValidTail = regexp.MustCompile(`(?is)\bNOT\s+VALID\s*$`)

func TestAConstraintOnAPreExistingTableIsAddedNotValid(t *testing.T) {
	// Read off the embedded tree rather than listed, so a namespace added later is
	// covered without anybody remembering this file.
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
			// A namespace with no baseline would take the zero value "" and arm its
			// whole backlog at once, which reads as this gate breaking rather than
			// as a decision somebody owes.
			if _, set := plainConstraintBaseline[namespace]; !set {
				t.Fatalf("namespace %q has no plainConstraintBaseline: pick the version this "+
					"obligation starts at for it, in the same change that adds the namespace", namespace)
			}
			dir, subErr := fs.Sub(files, namespace)
			if subErr != nil {
				t.Fatalf("reading the %s namespace: %v", namespace, subErr)
			}
			checkPlainConstraintScans(t, namespace, dir)
		})
	}
}

func checkPlainConstraintScans(t *testing.T, namespace string, dir fs.FS) {
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
		if version(path) < plainConstraintBaseline[namespace] {
			skipped++
			return nil
		}
		examined++
		for _, finding := range scansUnderExclusiveLock(string(body)) {
			t.Errorf("%s/%s adds %s to %s, which it did not create, without NOT VALID.\n"+
				"A plain ADD CONSTRAINT scans every existing row while holding ACCESS EXCLUSIVE, "+
				"so every write to that table waits for the scan. Add it NOT VALID here and "+
				"VALIDATE it in a LATER migration — a VALIDATE in this file would hold the same "+
				"lock for longer, which is what notvalidsplit_test.go refuses.",
				namespace, path, finding.constraint, finding.table)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the migration files: %v", err)
	}
	if examined == 0 && skipped == 0 {
		t.Errorf("%s: no .sql files at all — the embedded namespace is empty, so this gate "+
			"read nothing and passed", namespace)
	}
	t.Logf("%s: examined %d file(s), %d below the %s pin",
		namespace, examined, skipped, plainConstraintBaseline[namespace])
}

// scanningFinding is one constraint and the table it scans.
type scanningFinding struct {
	table      string
	constraint string
}

// scansUnderExclusiveLock reports every constraint a file adds to a table it did not
// create, without NOT VALID.
//
// The table comes from the ALTER that precedes the action, because one ALTER may
// carry several actions and they all name the same table.
func scansUnderExclusiveLock(sql string) []scanningFinding {
	statements := executableSQL(sql)
	own := tablesCreatedIn(statements)
	var found []scanningFinding
	for _, m := range scanningConstraint.FindAllStringSubmatchIndex(statements, -1) {
		table, ok := tableAlteredBefore(statements, m[0], alteredTable)
		if !ok {
			continue // an ADD CONSTRAINT inside a CREATE TABLE, which creates the table
		}
		// Created EARLIER in this same file: no rows, nobody waiting.
		if at, isOwn := own[table]; isOwn && at < m[0] {
			continue
		}
		if notValidTail.MatchString(strings.TrimSpace(statements[m[1]:actionEndsAt(statements, m[1])])) {
			continue
		}
		found = append(found, scanningFinding{
			table:      table,
			constraint: strings.Trim(statements[m[2]:m[3]], `"`),
		})
	}
	return found
}

// actionEndsAt finds where one ALTER action stops: the next comma or semicolon at
// depth zero.
//
// By DEPTH, because a CHECK body holds commas and parentheses of its own —
// `CHECK (x IN (1, 2))` — and stopping at the first comma would cut the action in
// half and lose a NOT VALID that follows.
func actionEndsAt(statements string, from int) int {
	depth := 0
	for i := from; i < len(statements); i++ {
		switch statements[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',', ';':
			if depth == 0 {
				return i
			}
		}
	}
	return len(statements)
}

// tableAlteredBefore answers which table the ALTER nearest above this offset names.
func tableAlteredBefore(statements string, at int, pattern *regexp.Regexp) (string, bool) {
	var table string
	var found bool
	for _, m := range pattern.FindAllStringSubmatchIndex(statements[:at], -1) {
		table = strings.ToLower(strings.Trim(statements[m[2]:m[3]], `"`))
		found = true
	}
	return table, found
}

// TestThePlainConstraintGateReportsWhatItClaimsTo probes the reading directly.
//
// Both namespaces sort below the pin today, so the walk above examines nothing and
// would pass whatever this logic did. These cases are what make the rule provable
// before the first migration arms it — without them the gate is a claim with no
// evidence, which is worse than no gate because the next reader trusts it.
func TestThePlainConstraintGateReportsWhatItClaimsTo(t *testing.T) {
	for _, c := range []struct {
		name  string
		sql   string
		finds int
	}{{
		name:  "plain CHECK on a table it did not create",
		sql:   `SET LOCAL lock_timeout = '3s'; ALTER TABLE activity ADD CONSTRAINT a_ck CHECK (x > 0);`,
		finds: 1,
	}, {
		name:  "NOT VALID CHECK on a table it did not create",
		sql:   `ALTER TABLE activity ADD CONSTRAINT a_ck CHECK (x > 0) NOT VALID;`,
		finds: 0,
	}, {
		name:  "plain FOREIGN KEY on a table it did not create",
		sql:   `ALTER TABLE activity ADD CONSTRAINT a_fk FOREIGN KEY (deal_id) REFERENCES deal(id);`,
		finds: 1,
	}, {
		// Postgres accepts ONLY and it alters the same table. Reading past it would
		// leave every such statement uninspected.
		name:  "ALTER TABLE ONLY",
		sql:   `ALTER TABLE ONLY activity ADD CONSTRAINT a_ck CHECK (x > 0);`,
		finds: 1,
	}, {
		// One ALTER, two actions: the first one's NOT VALID says nothing about the
		// second, and a tail that ran to the `;` would read it as covering both.
		name:  "multi-action ALTER where only the first is NOT VALID",
		sql:   `ALTER TABLE activity ADD CONSTRAINT c1 CHECK (x > 0) NOT VALID, ADD CONSTRAINT c2 CHECK (y > 0);`,
		finds: 1,
	}, {
		name:  "multi-action ALTER where both are NOT VALID",
		sql:   `ALTER TABLE activity ADD CONSTRAINT c1 CHECK (x > 0) NOT VALID, ADD CONSTRAINT c2 CHECK (y > 0) NOT VALID;`,
		finds: 0,
	}, {
		// No rows to scan and nobody waiting: the pattern would be a ritual here.
		name:  "plain CHECK on a table created earlier in the same file",
		sql:   `CREATE TABLE fresh (x int); ALTER TABLE fresh ADD CONSTRAINT f_ck CHECK (x > 0);`,
		finds: 0,
	}, {
		// IF NOT EXISTS creates nothing when the table is already there, so it
		// cannot establish that the rows the ALTER scans are this file's.
		name:  "plain CHECK after CREATE TABLE IF NOT EXISTS",
		sql:   `CREATE TABLE IF NOT EXISTS activity (x int); ALTER TABLE activity ADD CONSTRAINT a_ck CHECK (x > 0);`,
		finds: 1,
	}, {
		// POSITION, not presence: the ALTER runs against the live table.
		name:  "plain CHECK before the CREATE that names the same table",
		sql:   `ALTER TABLE fresh ADD CONSTRAINT f_ck CHECK (x > 0); DROP TABLE fresh; CREATE TABLE fresh (x int);`,
		finds: 1,
	}, {
		// UNIQUE builds an index rather than validating a predicate, and Postgres
		// does not accept NOT VALID for it — demanding one would be unfollowable.
		name:  "UNIQUE on a table it did not create",
		sql:   `ALTER TABLE activity ADD CONSTRAINT a_uq UNIQUE (slug);`,
		finds: 0,
	}, {
		// A CHECK body nests parentheses, and the NOT VALID is past them. Reading
		// the tail with a pattern stopped early here and reported a COMPLIANT
		// constraint — blocking a correct migration, which is the worse direction
		// for this gate to fail in.
		name:  "nested CHECK body before NOT VALID",
		sql:   `ALTER TABLE activity ADD CONSTRAINT a_ck CHECK ((x > 0) AND (y > 0)) NOT VALID;`,
		finds: 0,
	}, {
		name:  "nested CHECK body with no NOT VALID",
		sql:   `ALTER TABLE activity ADD CONSTRAINT a_ck CHECK ((x > 0) AND (y > 0));`,
		finds: 1,
	}, {
		// Commas inside the body are not action separators.
		name:  "a CHECK whose body holds a comma, then NOT VALID",
		sql:   `ALTER TABLE activity ADD CONSTRAINT a_ck CHECK (x IN (1, 2)) NOT VALID;`,
		finds: 0,
	}, {
		name:  "a NOT VALID that is only in a comment",
		sql:   "ALTER TABLE activity ADD CONSTRAINT a_ck CHECK (x > 0); -- NOT VALID\n",
		finds: 1,
	}, {
		// A constraint written INSIDE a CREATE TABLE scans nothing: the table is
		// coming into existence with it.
		name:  "a constraint declared inside CREATE TABLE",
		sql:   `CREATE TABLE fresh (x int, CONSTRAINT f_ck CHECK (x > 0));`,
		finds: 0,
	}} {
		t.Run(c.name, func(t *testing.T) {
			found := scansUnderExclusiveLock(c.sql)
			if len(found) != c.finds {
				t.Errorf("read %d finding(s) %v, want %d: %s", len(found), found, c.finds, c.sql)
			}
		})
	}
}
