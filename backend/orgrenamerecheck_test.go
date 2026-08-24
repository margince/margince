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

// withoutSQLNoise removes what a reader can see is not the statement: line
// comments and single-quoted literals.
//
// Both directions were wrong without it. `SET description = $1 -- display_name =`
// counted as a rename, and `SET description = 'legal_name = x'` did too; the
// other way, a `;` INSIDE a comment ended the `[^;]*?` scan early and hid a real
// `legal_name = $2` on the next line. A `--` inside a quoted literal is handled
// by stripping quotes first; an escaped quote (`”`) closes and reopens, which
// is the same result for this purpose.
func withoutSQLNoise(statement string) string {
	statement = sqlQuoted.ReplaceAllString(statement, "''")
	return sqlLineComment.ReplaceAllString(statement, "")
}

// sqlQuoted is a single-quoted literal's body. The comment half of the job is
// versionguard_test.go's sqlLineComment, which already strips `-- …` for the
// same reason and is in this package — a second spelling of it here would be
// two answers to one question.
var sqlQuoted = regexp.MustCompile(`'[^']*'`)

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

	var findings []string
	writers, named, assembled := 0, 0, 0
	for name, entry := range graph {
		if name == renameRecheck {
			continue
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
