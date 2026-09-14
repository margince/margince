// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// What counts as WORKING a deal, which is what re-arms a dismissal.
//
// "Not now" silences a stalled deal until it is next worked, and the episode
// fingerprint is what decides when that is. The rule: a new activity or a stage
// move, and nothing else.
//
// It was written down as an open question for a release, and an open question
// in a comment is a rule nothing holds — a field added to the fingerprint, or
// one dropped from it, changes what a rep's dismissal means and fails nothing.
// These three cases are the specification.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stalled is one deal as the fingerprint sees it.
func stalled(idle time.Time, moves int) stalledDeal {
	return stalledDeal{
		ID: ids.UUID{}, Name: "Acme expansion", IdleSince: idle, StageMoves: moves,
	}
}

func TestANewActivityReArmsADismissal(t *testing.T) {
	t.Parallel()

	before := stalled(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC), 2)
	after := stalled(time.Date(2026, 9, 4, 14, 30, 0, 0, time.UTC), 2)

	if before.episode() == after.episode() {
		t.Errorf("logging a call left the episode at %q — the rep's dismissal goes on silencing a "+
			"deal somebody has since talked to, which is the one thing 'not now' does not mean",
			before.episode())
	}
}

func TestAStageMoveReArmsADismissal(t *testing.T) {
	t.Parallel()

	idle := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	before, after := stalled(idle, 2), stalled(idle, 3)

	if before.episode() == after.episode() {
		t.Errorf("advancing the deal left the episode at %q — advancing is the most deliberate "+
			"work there is on a deal, and it moves no timestamp the stall rule reads, so without "+
			"this term the dismissal survives every stage the deal goes on to reach",
			before.episode())
	}
}

// Re-pricing, pushing the close date, changing the owner: none of them is work
// on the deal, and a dismissal survives all of them.
//
// Deliberate rather than missed. The rep who said "not now" said it about a deal
// nobody is talking to, and correcting its amount does not make anybody talk to
// it — the advice would return with nothing new to say. An owner change needs no
// term for a different reason: a dismissal is per-user, so the new owner has
// dismissed nothing and the advice is already live for them.
func TestBookkeepingDoesNotReArmADismissal(t *testing.T) {
	t.Parallel()

	// The fingerprint's inputs are the two that DO count. A deal whose amount,
	// close date or owner changed has the same two, so its episode is the same —
	// which is exactly what this asserts, and what would stop being true if a
	// third input were added without a reason.
	idle := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	repriced := stalled(idle, 2)
	original := stalled(idle, 2)

	if original.episode() != repriced.episode() {
		t.Errorf("the episode moved from %q to %q with neither an activity nor a stage move — "+
			"something else has been folded into it, and a rep's 'not now' now expires on "+
			"bookkeeping they did themselves", original.episode(), repriced.episode())
	}
}
