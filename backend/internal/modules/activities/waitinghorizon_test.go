// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"
)

func days(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

// The whole point of deriving: an agency and an enterprise seller stop getting
// the same number. One compiled ninety judged both, and it was wrong in
// opposite directions — a queue full of forgotten conversations for one, a
// queue dropping live business for the other.
func TestTheHorizonFollowsWhatTheInstallationActuallyDoes(t *testing.T) {
	fast := derivedWaitingHorizonDays(waitingHorizonSpread{Slow: days(2), Answered: 500})
	slow := derivedWaitingHorizonDays(waitingHorizonSpread{Slow: days(40), Answered: 500})

	if fast >= waitingHorizonDays {
		t.Errorf("an installation whose slow answers take two days derives %d days, which is no shorter "+
			"than the compiled %d — the agency case is unserved", fast, waitingHorizonDays)
	}
	if slow <= waitingHorizonDays {
		t.Errorf("an installation whose slow answers take forty days derives %d days, which is no longer "+
			"than the compiled %d — the enterprise case is unserved", slow, waitingHorizonDays)
	}
	if fast >= slow {
		t.Errorf("the faster installation derived %d and the slower %d — the horizon does not follow the "+
			"behaviour it is measured from", fast, slow)
	}
}

// Too little to say, so it does not say. A fresh installation has no history,
// and a handful of conversations that happen to be slow is not a cadence.
func TestATooThinSampleKeepsTheCompiledHorizon(t *testing.T) {
	for _, spread := range []waitingHorizonSpread{
		{},
		{Slow: days(40), Answered: waitingHorizonMinSample - 1},
		// Answered enough, but nothing measured — the percentile came back zero
		// because every answer landed in the same instant it was asked, which is
		// a shape to refuse rather than to derive a same-day horizon from.
		{Slow: 0, Answered: 500},
	} {
		if got := derivedWaitingHorizonDays(spread); got != waitingHorizonDays {
			t.Errorf("a spread of %+v derived %d days, want the compiled %d", spread, got, waitingHorizonDays)
		}
	}
}

// Both bounds, and each is a failure mode rather than a tidiness rule: with no
// ceiling the derivation answers "keep everything forever", which is not a
// queue; with no floor an installation that answers within the hour drops a
// customer who waited a week.
func TestTheDerivationRefusesToProduceAQueueThatIsNotOne(t *testing.T) {
	if got := derivedWaitingHorizonDays(waitingHorizonSpread{Slow: days(900), Answered: 500}); got != waitingHorizonCeilingDays {
		t.Errorf("an installation with very slow answers derived %d days, want the ceiling %d — a horizon "+
			"that keeps everything forever is the queue's other failure mode", got, waitingHorizonCeilingDays)
	}
	if got := derivedWaitingHorizonDays(waitingHorizonSpread{Slow: time.Hour, Answered: 500}); got != waitingHorizonFloorDays {
		t.Errorf("an installation answering within the hour derived %d days, want the floor %d — a week's "+
			"silence is not history in any business", got, waitingHorizonFloorDays)
	}
}

// Zero is the field's zero value, not a horizon, and reading it as one would
// call every wait history and empty the queue.
//
// It is reachable rather than defensive: ListActivitiesTx is a public seam for
// a caller that already holds a transaction, so it has no store to measure
// through — and a caller that sets WaitingReplyAsOf on it hands this an
// unmeasured horizon by construction. The honest answer for that path is
// today's behaviour.
func TestAnUnmeasuredHorizonIsTheCompiledOneAndNotNoDaysAtAll(t *testing.T) {
	for _, unmeasured := range []int{0, -1} {
		if got := horizonOrDefault(unmeasured); got != waitingHorizonDays {
			t.Errorf("horizonOrDefault(%d) = %d, want the compiled %d — a horizon of no days calls every "+
				"wait history and empties the queue", unmeasured, got, waitingHorizonDays)
		}
	}
	// And a measured one is passed through, or the guard would pin every
	// installation back to ninety and the derivation would reach nothing.
	if got := horizonOrDefault(21); got != 21 {
		t.Errorf("horizonOrDefault(21) = %d, want 21 — the guard is swallowing the measurement", got)
	}
}
