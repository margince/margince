// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// A company's NAME is the axis on which two records of one company converge, so
// every rename has to ask whether it just created a duplicate.
//
// `recheckOrgNameForDuplicates` is called from a handful of places that each had
// to remember the rule on their own, and a comment on one of them said it was
// "the only writer of that column a human drives" — which told the next author
// the question was settled. A rename that forgot would leave two records of one
// company sitting beside each other with nothing to notice.
//
// This holds the invariant instead of the claim: a function that can reach a
// statement setting `organization.display_name` or `organization.legal_name`
// must be able to reach the re-check. That is derived from the tree rather than
// maintained as a list, which is the point — a NEW writer is a finding on the
// day it is written, without anybody remembering this file exists.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

const (
	peoplePackage    = "internal/modules/people"
	renameRecheck    = "recheckOrgNameForDuplicates"
	organizationName = "organization.display_name / organization.legal_name"
)

// setsAnOrganizationName matches a statement that moves a name column.
//
// An UPDATE only. A CREATE is a different question and is answered elsewhere:
// every path to the one `INSERT INTO organization` (in `createOrganization`)
// runs the PO-F-2 match first and refuses on a collision —
// `DedupeOrganizationForCreate`, `resolveOrCreateAnchor` and
// `manualDedupeOrganization` are the three that feed it. A row that reaches the
// INSERT has therefore already been compared against every existing name, and
// asking it to re-check afterwards would be asking it to compare a row with
// itself.
var setsAnOrganizationName = regexp.MustCompile(`(?is)UPDATE\s+organization\b[^;]*?\b(display_name|legal_name)\s*=`)

// assemblesAnOrganizationUpdate matches an UPDATE whose COLUMN is a variable,
// so the statement names no column for the pattern above to find.
//
// This is not hypothetical: organization_profile_field_write.go writes
// `UPDATE organization SET ` + column + ` = $2`, and drives it with
// "display_name" and "legal_name". The census could not see it as a writer at
// all — so the promise that a new writer is a finding on the day it is written
// was false for a shape the tree already contained, and the next author copies
// what is there.
//
// A fragment ending at SET is judged like a named write: the gate cannot know
// which column arrives, so it asks the same question it asks of one it can read.
var assemblesAnOrganizationUpdate = regexp.MustCompile(`(?is)UPDATE\s+organization\b[^;]*?\bSET\s*$`)

// withoutSQLNoise blanks every region of a statement the database does not
// execute: quoted literals, dollar-quoted bodies, line comments and block
// comments.
//
// ONE left-to-right scanner, because each of those regions hides the others'
// delimiters, and a pass cannot see past what a later pass has not removed yet.
// Every arrangement of separate passes is wrong in some direction:
//
//	UPDATE organization -- /*
//	SET legal_name = $1                        a `/*` inside a line comment opens nothing
//	UPDATE organization SET description = 'x' -- don't
//	SET legal_name = $1                        an apostrophe inside a comment closes nothing
//	UPDATE organization /* ; */ SET legal_name = $1
//	                                           a `;` inside any of them ends nothing
//
// Each region knows only its own terminator, so the scanner tracks which one it
// is in rather than asking four patterns what they can each see on their own.
func withoutSQLNoise(statement string) string {
	var out strings.Builder
	for i := 0; i < len(statement); {
		tag := dollarTagAt(statement, i)
		switch {
		case statement[i] == '\'':
			out.WriteString("''")
			i = endOfQuoted(statement, i)
		case tag != "":
			out.WriteString("$$")
			i = endOfDollarQuoted(statement, i, tag)
		// Either comment leaves a space behind it: a comment is whitespace to
		// the parser, and what surrounded it must not fuse. `SET/* x */legal_name`
		// is two tokens, not one.
		case strings.HasPrefix(statement[i:], "--"):
			out.WriteByte(' ')
			i = endOfLineComment(statement, i)
		case strings.HasPrefix(statement[i:], "/*"):
			out.WriteByte(' ')
			i = endOfBlockComment(statement, i)
		default:
			out.WriteByte(statement[i])
			i++
		}
	}
	return out.String()
}

// endOfQuoted returns the index just past a `'…'` literal beginning at i.
//
// `”` — an escaped quote — needs no case of its own, which is worth saying
// because its absence reads like the bug it usually is. Every quote is a
// boundary either way, so the two readings of `'it”s'` (one literal, or two
// abutting ones) blank exactly the same bytes, and this scanner only ever asks
// which bytes are literal. A case for it here would be code no probe can fail.
func endOfQuoted(statement string, i int) int {
	for j := i + 1; j < len(statement); j++ {
		if statement[j] == '\'' {
			return j + 1
		}
	}
	return len(statement)
}

// dollarTagAt returns the `$tag$` opening at i, or "" if there is none. A body
// closes only on its OWN tag, so `$$…$$` and `$q$…$q$` do not close each other.
func dollarTagAt(statement string, i int) string {
	if statement[i] != '$' {
		return ""
	}
	for j := i + 1; j < len(statement); j++ {
		if statement[j] == '$' {
			return statement[i : j+1]
		}
		// A tag is an identifier. A leading DIGIT means this is a placeholder
		// instead, and `$1 … $2` is two parameters and not a body between them.
		digitInside := j > i+1 && '0' <= statement[j] && statement[j] <= '9'
		if !isTagLetter(statement[j]) && !digitInside {
			return ""
		}
	}
	return ""
}

func isTagLetter(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func endOfDollarQuoted(statement string, i int, tag string) int {
	body := i + len(tag)
	closes := strings.Index(statement[body:], tag)
	if closes < 0 {
		return len(statement)
	}
	return body + closes + len(tag)
}

// endOfLineComment stops AT the newline rather than past it, so the line break
// stays in the stream. It is whitespace, and removing it fuses the comment's
// line to the next one.
func endOfLineComment(statement string, i int) int {
	ends := strings.IndexByte(statement[i:], '\n')
	if ends < 0 {
		return len(statement)
	}
	return i + ends
}

// endOfBlockComment counts NESTING, because Postgres nests `/* … */` where C
// does not: a non-greedy match stops at the first `*/`, leaving the tail of
// `/* outer /* inner */ still comment */` in the stream to be read as SQL.
func endOfBlockComment(statement string, i int) int {
	depth := 0
	for j := i; j < len(statement); j++ {
		switch {
		case strings.HasPrefix(statement[j:], "/*"):
			depth++
			j++
		case strings.HasPrefix(statement[j:], "*/"):
			depth--
			if depth == 0 {
				return j + 2
			}
			j++
		}
	}
	return len(statement)
}

// remembersTheRecheckItself ratifies a writer that cannot reach the re-check
// through an edge this graph can follow.
//
// Empty, and that is the finding rather than an oversight: every writer in the
// tree today reaches it. The map exists so that adding an entry is a visible
// decision with a reason beside it, instead of a silent edit to the pattern
// above — which is how a gate stops meaning anything.
var remembersTheRecheckItself = gatekit.Waive(map[string]string{})

func TestEveryOrganizationRenameReachesTheDuplicateRecheck(t *testing.T) {
	// A ratification that stops matching covers a writer that has moved or been
	// fixed, and leaving it in place quietly re-exempts whatever takes its name.
	defer remembersTheRecheckItself.AssertAllMatched(t)

	graph := packageCallGraph(t, peoplePackage)
	if _, known := graph[renameRecheck]; !known {
		t.Fatalf("%s is not in the graph, so every writer would trivially fail to reach it — "+
			"the re-check has been renamed or moved out of %s", renameRecheck, peoplePackage)
	}

	var findings, unreadable []string
	writers, named, assembled := 0, 0, 0
	for name, entry := range graph {
		if name == renameRecheck {
			continue
		}
		if statement, hiddenRename := firstRenameStatement(entry.hidden); hiddenRename {
			unreadable = append(unreadable, fmt.Sprintf("%s\n      %s", name, statement))
		}
		statement, renames := firstRenameStatement(entry.statements)
		if !renames {
			continue
		}
		writers++
		if assemblesAnOrganizationUpdate.MatchString(withoutSQLNoise(statement)) {
			assembled++
		} else {
			named++
		}
		if guardedBy(graph, name, renameRecheck) || remembersTheRecheckItself.Waived(t, name) {
			continue
		}
		findings = append(findings, fmt.Sprintf("%s\n      %s", name, statement))
	}

	// A census that judged nothing certifies nothing. The floor sits below the
	// real count so it catches a broken walk rather than a changing tree.
	// Two floors, not one, because a single count hides which half of the walk
	// broke. Deleting the package-level statement folding leaves three direct
	// writers standing — enough to clear any one floor while an entire route to
	// a statement has gone silent.
	if named < 4 || assembled < 1 {
		t.Fatalf("this census saw %d named-column writer(s) and %d assembled-column writer(s) "+
			"(%d in total); it expects at least 4 and 1, so one of the two ways a statement "+
			"reaches a function has stopped working rather than the tree having changed",
			named, assembled, writers)
	}
	if len(unreadable) > 0 {
		t.Errorf("these functions declare a local with the same name as a package-level "+
			"statement that renames an organization, so this census could not tell which one "+
			"they read:\n    %s\n\n"+
			"Suppression here is per FUNCTION rather than per block, which is the direction "+
			"that misses a writer — so it is reported instead of assumed harmless. Rename the "+
			"local, or rename the package value.",
			strings.Join(unreadable, "\n    "))
	}
	if len(findings) > 0 {
		t.Errorf("these functions rename an organization, and no route to them calls %s:\n    %s\n\n"+
			"A name is the axis on which two records of one company converge — PO-F-2 has nothing to "+
			"compare until one is filled in. A rename that skips the re-check leaves the duplicate it "+
			"just created with nothing to notice it. Call the re-check, or ratify the writer here with "+
			"the reason it does not need to.",
			renameRecheck, strings.Join(findings, "\n    "))
	}
}

// firstRenameStatement returns the statement that moves a name column, so the
// report points at the statement rather than dumping every string the function
// can reach.
func firstRenameStatement(statements []string) (string, bool) {
	for _, statement := range statements {
		readable := withoutSQLNoise(statement)
		if setsAnOrganizationName.MatchString(readable) ||
			assemblesAnOrganizationUpdate.MatchString(readable) {
			return strings.Join(strings.Fields(statement), " "), true
		}
	}
	return "", false
}

// The detector reads STATEMENTS, and a statement carries prose. These are the
// shapes that fooled it, kept because the census above is a census of zero once
// the tree is clean: it reads the same over a clean tree and over a detector
// that has stopped detecting.
func TestTheRenameDetectorReadsSQLAndNotProse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sql    string
		reads  bool
		reason string
	}{
		{
			"a semicolon inside a block comment", "UPDATE organization /* ; */ SET legal_name = $1", true,
			"the scan stopped at a semicolon that ends nothing, so a real rename went unseen",
		},
		{
			"a semicolon inside a line comment", "UPDATE organization -- ;\n SET legal_name = $1", true,
			"same, with the comment spelled the other way",
		},
		{
			"a semicolon inside a dollar-quoted body", "UPDATE organization SET legal_name = $tag$ ; $tag$", true,
			"a dollar-quoted literal is a body, not the end of a statement",
		},
		{
			"a column named only inside a comment", "UPDATE organization SET description = $1 -- legal_name =", false,
			"a rename mentioned in prose is not a rename, and reporting it teaches readers to skip this gate",
		},
		{
			"a column named only inside a string", "UPDATE organization SET description = 'legal_name = x'", false,
			"same, inside a literal the database stores rather than executes",
		},
		{
			"a nested block comment", "UPDATE organization /* outer /* inner */ still comment */ SET legal_name = $1", true,
			"Postgres nests block comments and a non-greedy regex does not; the tail was read as SQL",
		},
		{
			"a nested block comment hiding a column", "UPDATE organization SET description = $1 /* a /* b */ legal_name = */", false,
			"everything between the outer pair is comment, however many pairs are inside it",
		},
		{
			"the column assembled by the caller", "UPDATE organization SET ", true,
			"the fragment names no column because the caller supplies it; the gate asks anyway",
		},
		{
			"a block comment opened inside a line comment", "UPDATE organization -- /*\n SET legal_name = $1", true,
			"a `/*` the database reads as prose opened a comment that ran on and swallowed the rename after it",
		},
		{
			"an apostrophe inside a line comment", "UPDATE organization -- don't\n SET legal_name = $1", true,
			"an apostrophe in prose pairs with the next real quote and takes the SQL between them with it",
		},
		{
			"an apostrophe inside a block comment", "UPDATE organization /* don't */ SET legal_name = $1", true,
			"same, spelled the other way",
		},
		{
			"placeholders are not a dollar-quoted body", "UPDATE organization SET description = $1, legal_name = $2", true,
			"a tag is an identifier, so `$1 … $2` is two parameters and not a body hiding what is between them",
		},
		{
			"a body that does not close on a different tag", "UPDATE organization SET description = $q$ $$ legal_name = $q$", false,
			"`$$` does not close `$q$`, so the column named inside the body is still inside it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readable := withoutSQLNoise(tc.sql)
			got := setsAnOrganizationName.MatchString(readable) ||
				assemblesAnOrganizationUpdate.MatchString(readable)
			if got != tc.reads {
				t.Errorf("reads as an organization rename = %v, want %v — %s\n  raw:      %q\n  stripped: %q",
					got, tc.reads, tc.reason, tc.sql, readable)
			}
		})
	}
}
