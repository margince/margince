// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backoff_test

import (
	"math"
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/backoff"
)

const (
	base    = 2 * time.Minute
	ceiling = 4 * time.Hour
)

// The ladder is asserted as a BAND, because the jitter is the point: an exact
// assertion could only pass by removing the thing this package exists for.
func TestTheLadderDoublesOncePerPriorFailure(t *testing.T) {
	for _, tc := range []struct {
		priorFailures int
		want          time.Duration
	}{
		{0, base},
		{1, 2 * base},
		{2, 4 * base},
		{3, 8 * base},
	} {
		got := backoff.Jittered(tc.priorFailures, base, ceiling)
		if low, high := scaled(tc.want); got < low || got > high {
			t.Errorf("Jittered(%d) = %v, want within ±20%% of %v (%v..%v)",
				tc.priorFailures, got, tc.want, low, high)
		}
	}
}

// The first retry waits ONE interval, not two. priorFailures counts what has
// already gone wrong, and an off-by-one here doubles every wait in the product.
func TestTheFirstRetryWaitsOneInterval(t *testing.T) {
	got := backoff.Jittered(0, base, ceiling)
	if low, high := scaled(base); got < low || got > high {
		t.Errorf("Jittered(0) = %v, want one base interval (%v..%v)", got, low, high)
	}
}

func TestTheLadderStopsAtTheCeiling(t *testing.T) {
	// Far past the point where doubling would overflow a duration if the loop
	// did not stop: 2min doubled 60 times is beyond int64 nanoseconds, so a
	// ladder that kept going would come back negative.
	got := backoff.Jittered(60, base, ceiling)
	if low, high := scaled(ceiling); got < low || got > high {
		t.Errorf("Jittered(60) = %v, want the ceiling (%v..%v)", got, low, high)
	}
}

// The jitter has to actually vary. A constant multiplier would pass every band
// above while leaving a fleet retrying in lockstep — which is the failure this
// package exists to prevent, and the one an exact assertion cannot see.
func TestTheJitterSpreadsRepeatedCalls(t *testing.T) {
	seen := map[time.Duration]bool{}
	for range 50 {
		seen[backoff.Jittered(3, base, ceiling)] = true
	}
	if len(seen) < 10 {
		t.Errorf("50 calls produced %d distinct delays; the jitter is not spreading them, so a fleet "+
			"that failed together would retry together", len(seen))
	}
}

// And it stays INSIDE the band. A spread that reached zero would retry
// instantly against a provider that has just refused.
func TestTheJitterNeverLeavesTheBand(t *testing.T) {
	low, high := scaled(base)
	for range 200 {
		if got := backoff.Jittered(0, base, ceiling); got < low || got > high {
			t.Fatalf("Jittered(0) = %v, outside ±20%% of %v (%v..%v)", got, base, low, high)
		}
	}
}

// scaled is the ±20% band around a delay.
func scaled(delay time.Duration) (low, high time.Duration) {
	return time.Duration(float64(delay) * 0.8), time.Duration(float64(delay) * 1.2)
}

// A base that is not positive is a ladder nobody configured, and the negative
// case is the one that does damage: doubling walks AWAY from the ceiling, the
// jitter scales a negative duration, and a scheduler asked to wait it comes
// back instantly — against a system that has just failed. Both callers pass
// constants today, which is exactly why nothing would have caught it.
func TestABaseThatIsNotPositiveIsNoLadderAtAll(t *testing.T) {
	for _, base := range []time.Duration{0, -time.Minute} {
		for _, priorFailures := range []int{0, 1, 5} {
			if got := backoff.Jittered(priorFailures, base, ceiling); got != 0 {
				t.Errorf("Jittered(%d, %v, ceiling) = %v, want no wait at all",
					priorFailures, base, got)
			}
		}
	}
}

// A base past half the Duration range overflows on the double and comes back
// negative, and the clamp after the loop never sees a number to clamp. Absurd
// as a configuration, and the ladder still owes a positive answer: a scheduler
// asked to wait a negative interval retries instantly, against a system that
// has just failed.
func TestABaseNearTheDurationCeilingDoesNotOverflowIntoANegativeWait(t *testing.T) {
	huge := time.Duration(1) << 62
	roof := time.Duration(math.MaxInt64)
	for _, priorFailures := range []int{1, 2, 10} {
		got := backoff.Jittered(priorFailures, huge, roof)
		if got <= 0 {
			t.Errorf("Jittered(%d, 1<<62, MaxInt64) = %v; the double overflowed past the clamp",
				priorFailures, got)
		}
		if got > roof {
			t.Errorf("Jittered(%d, 1<<62, MaxInt64) = %v, above the ceiling", priorFailures, got)
		}
	}
}
