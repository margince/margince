// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// A company's NAME is the axis on which two records of one company converge, so
// every rename has to ask whether it just created a duplicate.
//
// `companyform.go` said it was "the only writer of that column a human drives".
// Its two callers are the ones it names — the company form and a site-read
// confirmation — but it is not the only writer, and the claim invited the next
// author to stop looking. `recheckOrgNameForDuplicates` has SEVEN independent
// call sites, each of which had to remember the rule on its own; a rename that
// forgot would leave two records of one company sitting beside each other with
// nothing to notice.
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
// `resolveOrCreateOrganization` runs the dedupe match BEFORE it inserts and
// refuses on a collision, so a row that reaches the INSERT has already been
// compared against every existing name. Asking it to re-check afterwards would
// be asking it to compare a row with itself.
var setsAnOrganizationName = regexp.MustCompile(`(?is)UPDATE\s+organization\b[^;]*?\b(display_name|legal_name)\s*=`)

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
	writers := 0
	for name, entry := range graph {
		if name == renameRecheck {
			continue
		}
		statement, renames := firstRenameStatement(entry.statements)
		if !renames {
			continue
		}
		writers++
		if guardedBy(graph, name, renameRecheck) || remembersTheRecheckItself.Waived(t, name) {
			continue
		}
		findings = append(findings, fmt.Sprintf("%s\n      %s", name, statement))
	}

	// A census that judged nothing certifies nothing. The floor sits below the
	// real count so it catches a broken walk rather than a changing tree.
	if writers < 3 {
		t.Fatalf("only %d function(s) were seen to write %s, so this census covered almost nothing",
			writers, organizationName)
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
		if setsAnOrganizationName.MatchString(statement) {
			return strings.Join(strings.Fields(statement), " "), true
		}
	}
	return "", false
}
