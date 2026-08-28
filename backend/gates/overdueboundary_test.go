// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// "Is this late?" is one question about one record, and a reader can ask it of a
// list, a card, a brief or an agent tool. They must all answer alike, because
// the reader does not know which one they are looking at — and the answer they
// disagreed about was the boundary: whether a promise due at this instant has
// already been missed.
//
// It has not. At the moment a promise falls due it is due; a reader told they
// had failed something in the instant it came due has been told a thing that is
// not so. `shared/kernel/deadline` holds that reading, and the SQL asks it the
// same way with `due_at < now()`.
//
// WHAT THIS DOES NOT COVER, so the absence of findings is not read as a wider
// claim than it is: fields carrying a promise under another name — an SLA
// `deadline`, a deal's `expected_close_date` — and the frontend, which decides
// lateness in TypeScript no Go census reads.
//
// THE DISTINCTION THAT MADE THEM DRIFT, because a gate that does not name it
// will be read as forbidding both: a SCHEDULER asks whether the moment has
// ARRIVED — inclusive, since a job due now should run now — and an overdue
// display asks whether it has PASSED. Both are right for their own question.
// This census governs the second only, which is why it keys on a comparison
// against a due date rather than on `<=` anywhere.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// deadlineOwner is the package that may spell the comparison.
const deadlineOwner = "internal/shared/kernel/deadline"

// schedulerClaims ask whether the moment has ARRIVED, not whether it has
// passed, and are inclusive for that reason: a job due now is due now, and a
// scheduler that waited for the instant to go by would never run anything due
// exactly on a tick. In Go and in SQL alike — the question is the same one
// whichever language asks it.
//
// Ratified by name rather than excluded by a narrower pattern, because the two
// questions are written identically — `due_at <= now()` against `due_at <
// now()` — and nothing in the SQL says which is being asked. A pattern that
// could tell them apart would be a pattern that could be fooled.
var schedulerClaims = gatekit.Waive(map[string]string{
	"internal/compose/runnerservice.go":       "fires a cron-seeded agent when its scheduled moment has arrived. `!now.Before(due)` is the inclusive reading on purpose: a job due exactly on a tick must run on that tick, and nothing here is shown to a reader as late.",
	"internal/modules/agents/runner/store.go": "claims the next runner job whose scheduled moment has arrived. This is the scheduler's question, and a job due exactly now must run rather than wait a tick; nothing here is shown to a reader as late.",
})

// asksIfLateInSQL matches a statement asking whether a due column has been
// reached INCLUSIVELY — the reading no display wants.
//
// Both directions, because `now() >= due_at` is the same question with the
// operands swapped — the blindness the Go arm carried until a round ago.
//
// The other side must be a CLOCK: `now()`, `current_timestamp`, or a bound
// parameter, since this tree does not write the clock literally (`deals/health.go`
// binds it as `a.due_at < $2`, so a census keyed on `now()` could not have seen
// an inclusive version of the statement it was written to guard).
//
// Requiring a clock is also what keeps `due_at <= due_on` out. Two due dates
// compared to each other is finding the earlier of them — a different and
// legitimate question, permitted by the Go arm, and a census that refused it in
// SQL while allowing it in Go would be two rules for one decision.
var asksIfLateInSQL = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdue_at\s*<=\s*(now\(\)|current_timestamp|\$\d+)`),
	regexp.MustCompile(`(?i)(now\(\)|current_timestamp|\$\d+)\s*>=\s*[a-z_.]*\bdue_at\b`),
}

// TestTheSQLPatternsSeeWhatTheyClaimTo pins both what they catch and what they
// must let through.
//
// A census asserting a shape is ABSENT passes identically over a clean tree and
// over a pattern that has stopped matching.
func TestTheSQLPatternsSeeWhatTheyClaimTo(t *testing.T) {
	t.Parallel()
	caught := []string{
		"WHERE a.due_at <= now()",
		"WHERE a.due_at <= $2",
		"WHERE a.due_at <= current_timestamp",
		"WHERE now() >= a.due_at",
		"WHERE $3 >= due_at",
		"where DUE_AT  <=  NOW()",
	}
	for _, statement := range caught {
		if !anyAsksIfLate(statement) {
			t.Errorf("the census does not see an inclusive reading it must:\n\t%s", statement)
		}
	}
	missed := []string{
		// The exclusive reading, which is the one every surface wants.
		"WHERE a.due_at < now()",
		"WHERE a.due_at < $2",
		// Two due dates compared to each other: finding the earlier of them.
		"WHERE due_at <= due_on",
		"WHERE inv.due_at <= q.due_at",
		// A due date in the FUTURE is a different question again.
		"WHERE a.due_at >= now()",
		// Another column entirely.
		"WHERE expires_at <= now()",
	}
	for _, statement := range missed {
		if anyAsksIfLate(statement) {
			t.Errorf("the census reports something that is not an inclusive lateness test:\n\t%s", statement)
		}
	}
}

func anyAsksIfLate(statement string) bool {
	for _, inclusive := range asksIfLateInSQL {
		if inclusive.MatchString(statement) {
			return true
		}
	}
	return false
}

func TestOnlyOnePlaceDecidesWhetherSomethingIsLate(t *testing.T) {
	t.Parallel()
	// A ratification that stops matching is one for a statement that has moved
	// or been fixed, and leaving it re-exempts whatever takes its place.
	defer schedulerClaims.AssertAllMatched(t)

	var inGo, inSQL []string
	judged := 0
	fset := token.NewFileSet()
	for _, root := range []string{"internal", "../extensions"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "testdata" || name == "node_modules" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			rel := filepath.ToSlash(path)
			if strings.Contains(rel, deadlineOwner) {
				return nil
			}
			source, readErr := os.ReadFile(path) // #nosec G304 -- a *.go path from walking the trusted source tree
			if readErr != nil {
				return readErr
			}
			judged++
			for _, statement := range gatekit.SQLStatementsIn(t, path, string(source)) {
				if anyAsksIfLate(statement) && !schedulerClaims.Waived(t, rel) {
					inSQL = append(inSQL, rel+": "+gatekit.FirstLineOf(statement))
				}
			}
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || !comparesADueDateToTheClock(call) {
					return true
				}
				if !schedulerClaims.Waived(t, rel) {
					inGo = append(inGo, rel)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if judged < 500 {
		t.Fatalf("the census read only %d Go files, so it covered almost nothing", judged)
	}
	sort.Strings(inGo)
	sort.Strings(inSQL)
	if len(inGo) > 0 {
		t.Errorf("%d site(s) compare a due date to the clock themselves.\n\n"+
			"Whether a promise due at this instant is already late is one decision, and the "+
			"surfaces that disagreed about it were a list, a card and an agent tool answering the "+
			"same reader. Call deadline.Passed.\n\n\t%s", len(inGo), strings.Join(inGo, "\n\t"))
	}
	if len(inSQL) > 0 {
		t.Errorf("%d statement(s) ask whether a due moment has been reached INCLUSIVELY.\n\n"+
			"A row counted late in SQL and upcoming in Go is the same record answering two ways "+
			"depending on which surface assembled it. Use `<`.\n\n\t%s",
			len(inSQL), strings.Join(inSQL, "\n\t"))
	}
}

// comparesADueDateToTheClock reports whether a call compares a due date against
// the clock, in EITHER direction.
//
// `now.After(due)` is the same decision as `due.Before(now)` with the operands
// swapped, and reading only the receiver missed it — which is how a second
// spelling of "is this invoice late" lived in the same module as the first.
//
// Exactly one side must name a due date. Two due dates compared to each other is
// finding the earlier of them, a different and legitimate question; neither side
// naming one is not this decision at all.
func comparesADueDateToTheClock(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	if selector.Sel.Name != "Before" && selector.Sel.Name != "After" {
		return false
	}
	return mentionsADueDate(selector.X) != mentionsADueDate(call.Args[0])
}

// mentionsADueDate reports whether an expression names a DUE date.
//
// "due" only, and that is a scope decision rather than an oversight. A due date
// is a moment promised to a person, and being past it is something a reader is
// shown. An expiry, a lease, a quiesce deadline and a token lifetime are also
// moments compared against a clock, and none of them is a promise anybody
// missed — widening to those words reported eleven sites that were all correct,
// which is how a census teaches its readers to skip the output.
//
// Fields that carry a promise under another name — an SLA `deadline`, a deal's
// `expected_close_date` — are real and are NOT covered here. They are tracked
// rather than swept in, because each has its own semantics to settle first.
func mentionsADueDate(expr ast.Expr) bool {
	named := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && strings.Contains(strings.ToLower(ident.Name), "due") {
			named = true
		}
		return true
	})
	return named
}
