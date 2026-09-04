// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// capture.noiseJudgedStandsSQL calls itself the ONE spelling of "an
// already-settled answer disowns this contact", and says the scan and the write
// cannot drift into asking different questions. This is what holds that.
//
// The claim is worth holding because the two readers run in DIFFERENT
// transactions and the answer is destructive. The sweep selects a contact
// whose address a machine verdict settled as noise, or whose owner recorded a
// standing keep_out; the retraction then re-asks the same question on its own
// transaction before archiving the person. If the two ever spelled it
// differently — one forgetting that a live `pending` question outranks a stale
// noise row, say, or that an owner's keep_out claims only the decider's own
// record — the recheck would stop being a recheck: it would answer a question
// the scan never asked, and withdraw a contact on evidence nobody selected it
// for.
//
// What the gate judges: a SQL statement that PAIRS the two halves of that
// question — the keep_out override and a noise verdict on the pending ledger.
// Either half alone is a different, legitimate question, and the tree asks
// several of them: senderslist.go lists what a settled address did, and
// pendingsweeps.go and purgepersonal.go both read `status` for their own
// reasons. Only the conjunction IS this question, and only its owner may
// spell it.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// noiseVerdictOwner is where the definition lives. Its own statement is the
// definition rather than a copy of it.
const noiseVerdictOwner = "internal/modules/capture/strandedcontacts.go"

// noiseVerdictHelper is the spelling every other reader has to reach.
const noiseVerdictHelper = "noiseJudgedStandsSQL"

// keepOutOverride matches the owner's standing refusal: the override table
// under a keep_out decision. The decision constant is matched as the SQL
// literal it appears as, because a second spelling would be written in SQL.
var keepOutOverride = regexp.MustCompile(`capture_sender_override`)

// noiseLedgerVerdict matches a verdict read off the pending ledger — the
// other half of the question.
var noiseLedgerVerdict = regexp.MustCompile(`capture_pending_counterparty`)

// noiseDecision matches the settled answer itself, so a statement that merely
// joins the two tables for an unrelated reason is not mistaken for this one.
var noiseDecision = regexp.MustCompile(`'keep_out'`)

func TestTheNoiseVerdictQuestionHasOneSpelling(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	var findings []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		if filepath.ToSlash(path) == noiseVerdictOwner {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scope := helperScope{
			inside: file.Name != nil && file.Name.Name == "capture",
			names:  map[string]bool{noiseVerdictHelper: true},
		}
		for _, decl := range file.Decls {
			for _, sql := range noiseVerdictStatements(decl, scope, &judged) {
				findings = append(findings, fmt.Sprintf("%s: %s", filepath.ToSlash(path), firstNoiseLine(sql)))
			}
		}
	}
	// A census that judged nothing certifies nothing. The floor sits far below
	// the real count, so it catches a broken walk rather than a changing tree.
	if judged < 5 {
		t.Fatalf("only %d statement(s) reading capture's verdict tables were judged, so this census covered almost nothing", judged)
	}
	if len(findings) == 0 {
		return
	}
	t.Errorf("these statements ask capture's noise-verdict question in their own words:\n  %s\n\n"+
		"capture.%s is the one spelling, and the scan and the retraction both build from it. A second "+
		"copy makes the recheck answer a question the scan never asked — and the write it guards "+
		"archives somebody's contact. Call the helper.",
		strings.Join(findings, "\n  "), noiseVerdictHelper)
}

// noiseVerdictStatements returns the statements in a declaration that pair the
// override with the ledger verdict — this question, however it is worded.
//
// A statement rather than a literal, for the reason the employment-currency
// census gives: a query that calls the helper is a concatenation, and judging
// each literal on its own splits the question into halves that each look
// innocent. flattenSQL folds a concatenation into one text and contributes a
// helper CALL as its name, which is what makes the exemption real.
//
// judged counts every statement that reaches either table, so the floor above
// measures the walk rather than the verdict.
func noiseVerdictStatements(decl ast.Decl, capture helperScope, judged *int) []string {
	var out []string
	seen := map[ast.Node]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		if seen[n] {
			return false
		}
		text, ok := flattenSQL(n, seen, capture)
		if !ok {
			return true
		}
		touchesLedger := noiseLedgerVerdict.MatchString(text)
		if touchesLedger || keepOutOverride.MatchString(text) {
			*judged++
		}
		if touchesLedger && keepOutOverride.MatchString(text) && noiseDecision.MatchString(text) {
			out = append(out, text)
		}
		return true
	})
	return out
}

// firstNoiseLine names the offending statement by its first line naming either
// table, so a finding points at the query rather than printing all of it.
func firstNoiseLine(sql string) string {
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if noiseLedgerVerdict.MatchString(trimmed) || keepOutOverride.MatchString(trimmed) {
			return trimmed
		}
	}
	return strings.TrimSpace(sql)
}
