// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every state River declares is on one side of the sweep window or the other.
//
// activeSweepStates is what suppresses a duplicate tick, and it is read by every
// periodic pass in the product and by both push webhooks. Getting it wrong fails
// in two opposite directions: a state left out lets a second pass run while the
// first is still in flight, and a state wrongly included stops a sweep firing
// because something the window did not expect holds it open.
//
// So the list is checked against RIVER'S OWN vocabulary rather than against a
// copy of it. A ninth state — or a rename — fails here, where somebody decides
// which side it belongs on, instead of being discovered as a sweep that stopped.
//
// It is also the answer to the question this test was written for: whether a
// SNOOZED job is in flight. River has no snoozed state; JobSnooze puts the row
// back as `scheduled`, which the window already holds. Nothing to add, and this
// is what says so if that ever changes.

import (
	"testing"

	"github.com/riverqueue/river/rivertype"
)

// gatekit:fixture the states a run can be in HAVING STOPPED, and why each one is
// safe for the window to ignore. Not a waiver: nothing here excuses a finding —
// the map is one half of the classification this test checks River's vocabulary
// against, and an entry going stale means River dropped a state, which is the
// failure the test reports rather than something to sweep up.
//
// The window must not hold any of them open.
var finishedStates = map[rivertype.JobState]string{
	rivertype.JobStateCompleted: "the pass ran; the next scheduled tick is exactly what should follow it, " +
		"and River's default ByState would hold a 24h cadence shut until the row was cleaned out",
	rivertype.JobStateDiscarded: "the pass exhausted its attempts. A window held open by it would silence " +
		"the sweep for as long as the row survives, which is the failure that looks like nothing happening",
	rivertype.JobStateCancelled: "somebody stopped this run. The next tick is a new decision, not a duplicate of it",
}

func TestEveryRiverJobStateIsEitherInFlightOrFinished(t *testing.T) {
	t.Parallel()

	inFlight := map[rivertype.JobState]bool{}
	for _, state := range activeSweepStates {
		inFlight[state] = true
	}
	declared := rivertype.JobStates()
	if len(declared) == 0 {
		t.Fatal("river declares no job states — this census is reading nothing, and a census that " +
			"finds no subject cannot report a pass")
	}
	for _, state := range declared {
		_, finished := finishedStates[state]
		switch {
		case inFlight[state] && finished:
			t.Errorf("%q is listed as both in flight and finished: the window cannot both suppress "+
				"a tick and let it through", state)
		case !inFlight[state] && !finished:
			t.Errorf("river declares %q and neither activeSweepStates nor finishedStates names it. "+
				"Decide which it is: a state that means a pass is still coming belongs in the window, "+
				"or a second tick can be enqueued alongside the first; a state that means the run has "+
				"stopped must stay out, or the sweep goes quiet.", state)
		}
	}
}
