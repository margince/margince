// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
)

// What a top-level semicolon is, and what it is not.
//
// The reader beside this one replays whatever this returns against a real
// database, so a mis-split is not a wrong answer — it is a statement that never
// runs, or two that run as one and fail on syntax. Both read as "the migration
// is broken" from a very long way away.
//
// Every case here is a shape that hides a semicolon or a quote from a naive
// reading, and each one merged two statements before its arm existed.
func TestSemicolonsInsideQuotesAndCommentsDoNotSplitAStatement(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		sql  string
		want int
	}{
		"two plain statements":        {"SELECT 1; SELECT 2;", 2},
		"a semicolon in a literal":    {"SELECT 'a;b'; SELECT 2;", 2},
		"a doubled quote":             {"SELECT 'it''s; here'; SELECT 2;", 2},
		"a line comment":              {"SELECT 1 -- ; not a split\n; SELECT 2;", 2},
		"a block comment":             {"SELECT /* ; */ 1; SELECT 2;", 2},
		"nested block comments":       {"SELECT /* a /* ; */ b */ 1; SELECT 2;", 2},
		"a dollar-quoted body":        {"DO $$ BEGIN PERFORM 1; END $$; SELECT 2;", 2},
		"a tagged dollar-quoted body": {"DO $fn$ SELECT ';'; $fn$; SELECT 2;", 2},
		// An E-string's backslash escapes the byte after it, so the quote here
		// is INSIDE the literal and closes nothing.
		"an escaped quote in an E-string": {`SELECT E'a\'; b'; SELECT 2;`, 2},
		// And a plain literal's backslash escapes NOTHING —
		// standard_conforming_strings has been on by default since 9.1. Read as
		// an escape, the closing quote here is eaten and everything to the next
		// quote joins this statement.
		"a trailing backslash in a plain literal": {`SELECT 'C:\'; SELECT 2;`, 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := len(splitStatements(tc.sql)); got != tc.want {
				t.Errorf("split into %d statement(s), want %d:\n%s\n%q",
					got, tc.want, tc.sql, splitStatements(tc.sql))
			}
		})
	}
}

// isEStringPrefix reads the E as a TOKEN, not as the previous byte.
func TestOnlyAStandaloneEOpensAnEscapeString(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		sql  string
		want bool
	}{
		"a bare E":                {"E'x'", true},
		"an E after a space":      {"SELECT E'x'", true},
		"an E after a paren":      {"(E'x')", true},
		"no E at all":             {"'x'", false},
		"an identifier ending E":  {"THE'x'", false},
		"a digit before the E":    {"a1E'x'", false},
		"an underscore before it": {"_E'x'", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			quote := -1
			for i := 0; i < len(tc.sql); i++ {
				if tc.sql[i] == '\'' {
					quote = i
					break
				}
			}
			if quote < 0 {
				t.Fatalf("the fixture %q has no quote to ask about", tc.sql)
			}
			if got := isEStringPrefix(tc.sql, quote); got != tc.want {
				t.Errorf("isEStringPrefix(%q, %d) = %v, want %v", tc.sql, quote, got, tc.want)
			}
		})
	}
}

// What sqlWithoutComments takes out, and what it must not.
//
// It decides whether a migration is a CANDIDATE and whether a statement is a
// declaration, so a comment it leaves in makes prose look like a write, and a
// literal it eats makes a write look like prose.
func TestCommentsAreStrippedForClassificationAndLiteralsAreNot(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		sql  string
		want string
	}{
		"a line comment goes":      {"SELECT 1 -- permissions\nFROM role", "SELECT 1 \nFROM role"},
		"a block comment goes":     {"SELECT /* permissions */ 1", "SELECT  1"},
		"nested block comments go": {"SELECT /* a /* permissions */ b */ 1", "SELECT  1"},
		// The half that must NOT be stripped: a literal saying the same words is
		// what the migration writes, not what it says about itself.
		"a literal stays":            {"SELECT '-- permissions'", "SELECT '-- permissions'"},
		"a block-looking literal":    {"SELECT '/* permissions */'", "SELECT '/* permissions */'"},
		"a dollar-quoted body stays": {"DO $$ -- permissions\n$$", "DO $$ -- permissions\n$$"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := sqlWithoutComments(tc.sql); got != tc.want {
				t.Errorf("sqlWithoutComments(%q) = %q, want %q", tc.sql, got, tc.want)
			}
		})
	}
}

// A migration whose only mention of the column is prose is not a candidate.
//
// It used to be, and the hatch below could not save it: there is no statement to
// classify as a declaration, so it reached the fatal telling the reader to teach
// the pattern a spelling that does not exist.
func TestProseAboutPermissionsIsNotAWriteToThem(t *testing.T) {
	t.Parallel()
	const prose = "-- this migration leaves role.permissions alone\nALTER TABLE role ADD COLUMN note text;"
	if mentionsRolePermissions.MatchString(sqlWithoutComments(prose)) {
		t.Error("a migration that mentions the column only in a comment is read as touching it")
	}
	if !mentionsRolePermissions.MatchString(prose) {
		t.Fatal("the fixture no longer mentions the column at all, so it proves nothing")
	}
}

// And a declaration that opens with a block comment is still a declaration.
func TestACommentedDeclarationIsStillADeclaration(t *testing.T) {
	t.Parallel()
	const declared = "/* the grant matrix */ CREATE TABLE role (permissions jsonb NOT NULL);"
	if !onlyDeclaresPermissions(declared) {
		t.Error("a CREATE TABLE behind a block comment is not read as declaring the column — the " +
			"baseline is the one candidate that legitimately carries no write, and this is the " +
			"hatch it escapes through")
	}
}

// A write to another column is not a write to this one, whatever its comment
// says.
//
// splitStatements keeps comments, because what it returns is replayed against a
// real database — so the pattern has to be pointed at the stripped copy or it
// finds the word in prose and records a grant nobody made.
func TestACommentNamingTheColumnIsNotAWriteToIt(t *testing.T) {
	t.Parallel()
	const aside = "UPDATE role SET name = 'x' /* permissions handled elsewhere */;"
	if got := rolePermissionStatements(aside); len(got) != 0 {
		t.Errorf("a statement writing another column was recorded as a permissions write: %q", got)
	}
	const genuine = "UPDATE role SET permissions = '{}'::jsonb WHERE key = 'rep';"
	if got := rolePermissionStatements(genuine); len(got) != 1 {
		t.Errorf("the real write was not recorded (%q) — the case above would then pass against a "+
			"pattern that matches nothing at all", got)
	}
}
