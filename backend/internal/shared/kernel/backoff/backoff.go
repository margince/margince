// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package backoff is the one transient-failure ladder.
//
// Held by: TestTheJitteredLadderIsSpelledOnce (backend/onejitteredbackoff_test.go)
//
// It was spelled twice — the capture registry's and the overlay sweep's —
// character for character, down to the same //nolint comment on the same line.
// Two writers of one invariant, in sibling modules that cannot import each
// other, which is why the copy happened for an honest reason and why the home
// is kernel.
//
// The JITTER is the part that has to be shared. Doubling is arithmetic anyone
// would rewrite the same way; the ±20% spread is a decision, and it is the
// decision that stops a fleet which failed together from retrying together —
// one provider outage, every connection backing off on the identical schedule,
// and the recovery arriving as a thundering herd against a provider that has
// just come back up. A copy that drifted to ±5%, or dropped the jitter as
// noise, would look correct in review and would only be visible under an
// outage.
//
// The NUMBERS are deliberately the caller's. base and ceiling bound a ladder
// against one particular external system, and two systems that happen to agree
// today may not tomorrow — a provider with a tighter budget wants a longer
// floor, and that is a decision somebody makes rather than one this package
// makes for them. Each caller declares its own with its own reason.
package backoff

import (
	"math/rand/v2"
	"time"
)

// jitterSpread is the ±20%. Written as its two halves because that is how the
// multiplier is built: the low end, plus a random share of the full width.
const (
	jitterLow   = 0.8
	jitterWidth = 0.4
)

// Jittered is the delay after priorFailures consecutive failures: base doubled
// once per failure, capped at ceiling, spread ±20%.
//
// priorFailures is the count BEFORE this attempt, so zero returns the base —
// the first retry waits one interval rather than two.
func Jittered(priorFailures int, base, ceiling time.Duration) time.Duration {
	// A base that is not positive is a ladder nobody configured, and the
	// arithmetic below does something worse than nothing with it: doubling
	// zero stays zero, and doubling a NEGATIVE base walks away from the
	// ceiling, so the jitter returns a negative duration and a scheduler asked
	// to wait it retries immediately — against a system that has just failed.
	if base <= 0 {
		return 0
	}
	delay := base
	for i := 0; i < priorFailures && delay < ceiling; i++ {
		// Clamped BEFORE the double, not after. A base past half the Duration
		// range overflows on the multiply and comes back negative, and the
		// clamp below never sees a number to clamp — a scheduler asked to wait
		// a negative interval retries instantly, which is the failure this
		// whole ladder exists to prevent.
		if delay > ceiling/2 {
			delay = ceiling
			break
		}
		delay *= 2
	}
	if delay > ceiling {
		delay = ceiling
	}
	// G404 is about key material. This is scheduling noise whose whole purpose
	// is to be unpredictable to no one in particular — a cryptographic source
	// would cost entropy to buy a property nothing here needs.
	jitter := jitterLow + jitterWidth*rand.Float64() //nolint:gosec // G404: scheduling jitter, not key material — de-syncs a fleet that failed together
	return time.Duration(float64(delay) * jitter)
}
