// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// No tool may PROMISE a human in the loop that its tier does not put there.
//
// The governance line at the end of every description is derived from
// spec.Tier, and the admission gate enforces that same tier, so it is true by
// construction. The prose above it is hand-written and is not — and the two
// came apart. ADR-0055 stopped these verbs staging by default; the derived
// line followed the tier automatically; ten descriptions went on saying "a
// contact approves this call before it runs" directly above "Governance: runs
// immediately".
//
// The cost is not a stale sentence. A model reading send_email was told
// somebody would catch a mistaken send, and by default nobody would — so the
// surface understated its own authority to the one party deciding whether to
// use it, which is the opposite of what every other rule here is for.
//
// PHRASED AS CONTRADICTION, not as banned words. An installation may raise a
// verb to confirm-first, and a description that says so CONDITIONALLY is true
// at either tier and must stay legal. What must never appear is an
// unconditional promise under a tier that gives none.
//
// It lives in compose because only compose has the whole surface: a registry
// built in the agents package has no tools in it, so a census there would walk
// nothing and report PASS.

import (
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// promisesAHuman matches an UNCONDITIONAL claim that a contact answers the call
// before it takes effect.
var promisesAHuman = regexp.MustCompile(
	`(?i)a human approves (this|the) call|a human approves the (send|move)|` +
		`a human approves it (first|before)|once a human approves`)

// conditionallyStaged is the phrasing true at either tier: it says what happens
// where an installation HAS raised the verb, and promises nothing where it has
// not.
var conditionallyStaged = regexp.MustCompile(`(?i)where an installation has raised`)

func TestNoToolPromisesAnApprovalItsTierDoesNotRequire(t *testing.T) {
	t.Parallel()
	specs := servedSurface(t).Specs()
	if len(specs) == 0 {
		t.Fatal("the served surface offered no tools, so this census walked nothing")
	}
	checked := 0
	for _, spec := range specs {
		if spec.Tier != mcp.TierAutoExecute {
			continue
		}
		checked++
		sentence := promisesAHuman.FindString(spec.Description)
		if sentence == "" {
			continue
		}
		// A conditional clause elsewhere does not license an unconditional
		// promise — a caller reads the promise, not the qualification three
		// sentences away — but it is reported so a reader can see it was seen.
		qualified := ""
		if conditionallyStaged.MatchString(spec.Description) {
			qualified = "; it does also carry a conditional clause, which does not undo this"
		}
		t.Errorf("%s runs immediately and its description says %q%s — a caller is told a "+
			"contact will catch a mistake that nothing will catch",
			spec.Name, sentence, qualified)
	}
	// A census that stops reaching auto-execute tools reports PASS in the same
	// words as one with nothing left to fix.
	if checked < 20 {
		t.Fatalf("only %d auto-execute tools were read out of %d served — the walk has lost "+
			"part of the surface", checked, len(specs))
	}
}

// The conditional phrasing must stay legal, or the rule above collapses into
// "never mention approval" and strips the one sentence a confirm-first
// installation's caller needs.
func TestTheConditionalPhrasingIsNotMistakenForAPromise(t *testing.T) {
	t.Parallel()
	legal := "The lead is demoted when this call answers. Where an installation has raised this " +
		"verb to confirm first, the answer is a staged approval instead."
	if promisesAHuman.MatchString(legal) {
		t.Errorf("the conditional form reads as an unconditional promise, so no description "+
			"can state the truth at both tiers:\n%s", legal)
	}
	if banned := "A human approves this call before it runs."; !promisesAHuman.MatchString(banned) {
		t.Errorf("the pattern no longer catches the sentence it exists for:\n%s", banned)
	}
}
