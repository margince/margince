// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A producer that can be re-triggered remembers what a human refused.
//
// `StageUnlessDeclined` refuses to raise a proposal whose identity a human has
// already rejected. Plain `Stage` does not: `JoinPending` collapses a proposal
// that is still WAITING, and a rejection is precisely what stops it waiting. So
// a producer that runs again — a nightly sweep, a connector re-sync, a button a
// rep can press twice — puts the refused proposal straight back, and back again
// on every run after that.
//
// This is not a hypothesis. Four producers shipped with it: the nightly
// close-date sweep re-asked every morning, a captured merge came back on every
// connector cycle, a transcript proposal returned whenever somebody pressed read
// again, and a rate refresh re-offered a figure an admin had turned down on
// every click. In each case the rep learns that saying no does not stick, and in
// the merge case the pressure is toward an action that destroys the distinction
// between two records.
//
// So plain `Stage` is the exception here rather than the default, and an
// exception has to say which mechanism makes repetition impossible or intended.
// The waivers below are that record; each names the guard that settles it, in a
// form a reader can go and check.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// plainStageCall matches a staging through the engine's memoryless entry point.
// Anchored on the receiver-and-method shape rather than the bare word, so a
// mention in prose or a different Stage (files, workflows' own port) does not
// match.
var plainStageCall = regexp.MustCompile(`\b\w+\.Stage\(ctx, approvals\.StageInput\{`)

// stageWithoutMemory records, per file, why a plain Stage is right there.
//
// Each reason names the MECHANISM rather than restating the intent: "a human
// presses it" is a trigger shape, and the two entries that rest on that say so
// plainly instead of implying a guard they do not have.
var stageWithoutMemory = gatekit.Waive(map[string]string{
	"internal/compose/coldstart.go": "the only trigger is a person pasting text and pressing the button; " +
		"re-submitting after a rejection is them deliberately asking again, and refusing that would be the bug",

	"internal/compose/scrape.go": "the triggers are a person clicking read-this-website and the enrich tool at " +
		"page depth; both are a deliberate ask about one named company",

	"internal/compose/workflows.go": "automation claims its run first — workflow_run's " +
		"ON CONFLICT (handler, idempotency_key) DO NOTHING gates Apply, and a clock trigger's key is " +
		"derived from the anchor so re-evaluating an unchanged record folds; a rejection additionally " +
		"marks the run blocked",

	"internal/compose/captureverdictaccept.go": "the sweep's own backlog query excludes a decided offer " +
		"(AwaitingReview requires decided_at IS NULL), and ReconcileDeclined closes the ledger row a " +
		"rejection answered, so the row cannot come back round",
})

// stageCallFloor is how many plain-Stage call sites the tree holds today. Under-
// recognition is the one way this gate must not break — a scanner that matches
// nothing reports PASS over every producer at once — so this is the exact count
// rather than a margin below it.
const stageCallFloor = 4

func TestEveryRetriggerableProducerRemembersARefusal(t *testing.T) {
	t.Parallel()
	found := 0
	for path, body := range goSourcesUnder(t, "internal/compose") {
		for _, line := range strings.Split(body, "\n") {
			if !plainStageCall.MatchString(line) {
				continue
			}
			found++
			if stageWithoutMemory.Waived(t, filepath.ToSlash(path)) {
				continue
			}
			t.Errorf("%s stages through Stage, which has no rejection memory:\n  %s\n"+
				"a producer that can run again puts a refused proposal straight back, and "+
				"back again every run after. Use StageUnlessDeclined with an Identity naming "+
				"what makes two of its proposals the same question — or, if repetition here is "+
				"impossible or intended, waive the file in stageWithoutMemory with the mechanism "+
				"that settles it.", path, strings.TrimSpace(line))
		}
	}
	if found < stageCallFloor {
		t.Errorf("found %d plain Stage call(s) and expects at least %d — the scanner is "+
			"matching less than the tree holds, which reports PASS over producers nobody checked",
			found, stageCallFloor)
	}
	// A waiver that matched nothing describes code that has moved or been fixed,
	// and left standing it exempts whatever is written there next.
	stageWithoutMemory.AssertAllMatched(t)
}
