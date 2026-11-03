// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package relstrength is the relationship-strength arithmetic
// (formulas-and-rules §4): one deterministic recency × frequency ×
// reciprocity fold over counted interactions, never predictive ML.
//
// It is a leaf because the same arithmetic now answers two different
// questions. PO-F-3 asks how warm a contact is to the WORKSPACE, folding
// every activity linked to them. PO-F-3b asks how warm they are to ONE
// COLLEAGUE, folding only the interactions that colleague was actually in
// (ADR-0078). Same half-life, same saturation point, same reciprocity floor,
// and the same counting unit — two scores a user will see side by side on the
// same screen, so a constant tuned in one copy and not the other is a visible
// contradiction, not a rounding difference. The unit is on that list because
// the two gather from different tables — the workspace score from
// activity_link, the colleague score from activity_participant — and one unit
// across both is what keeps a chat-only relationship from reading as warm on
// the one and as never having spoken on the other.
// Held by: TestEveryInteractionCountReadsTheSharedUnit
// (backend/gates/interactionunit_test.go).
//
// The package holds no query and no storage. Callers gather counts however
// their question requires and hand them here; that separation is what lets
// the fold be tested against hand-computed values with no database at all.
package relstrength

import (
	"math"
	"time"
)

// The §4 tunables, with their spec parameter-registry names. One definition:
// PO-F-3 and PO-F-3b are the same curve over different inputs.
const (
	HalfLifeDays     = 30.0 // RELSTRENGTH_HALFLIFE_DAYS
	FreqSaturation   = 20.0 // RELSTRENGTH_FREQ_SATURATION
	ReciprocityFloor = 0.25 // RELSTRENGTH_RECIPROCITY_FLOOR
	WindowDays       = 90   // the frequency/reciprocity window
)

// Display buckets. A score is shown as a band, and BucketNone is the band for
// a relationship with no qualifying interaction at all — rendered as "no
// interactions yet" and never as a zero, because "we have never spoken" and
// "we spoke and it went cold" are different facts about an account.
const (
	BucketNone     = "none"
	BucketWeak     = "weak"
	BucketModerate = "moderate"
	BucketStrong   = "strong"
)

// Bucket thresholds, named rather than inline so the boundary a support
// conversation quotes is the boundary the code compares against.
const (
	strongAtLeast   = 60
	moderateAtLeast = 25
)

// Inputs are the counted facts the fold needs. The caller decides WHICH
// interactions counted — that choice is the whole difference between the
// workspace score and the per-colleague one — and this package does not
// second-guess it.
type Inputs struct {
	// LastTouch is the newest interaction of either direction, nil when there
	// has never been one.
	LastTouch *time.Time
	// LastEngaged is the newest interaction the contact took part in: a
	// message they sent, or a meeting or call that took place. Recency counts
	// from it, so our own unanswered send does not refresh it. Nil when they
	// never took part.
	LastEngaged *time.Time
	// Count90d is every qualifying interaction in the window; Inbound90d and
	// Outbound90d are the directed subset. They are not required to sum to
	// Count90d: an interaction with no recorded direction counts toward
	// frequency but cannot speak to reciprocity.
	Count90d    int
	Inbound90d  int
	Outbound90d int
	// LateCount90d is the part of Count90d after LastEngaged, all of it when
	// LastEngaged is nil; LateOutbound90d is its outbound part. Nothing the
	// contact did not answer counts toward the score: an unanswered send moves
	// neither frequency nor reciprocity. They stay in the counts a surface
	// prints.
	LateCount90d    int
	LateOutbound90d int
}

// Score is the explainable output: the 0–100 number, its band, and the three
// factors it reconciles to exactly. The factors are returned, not just used,
// because a score a user cannot decompose is a mystery number (P6).
type Score struct {
	Strength    int
	Bucket      string
	Recency     float64
	Frequency   float64
	Reciprocity float64
}

// Compute folds the inputs through §4 at the given instant.
//
// It is pure: same inputs, same clock, same answer, with no hidden read of
// the wall clock. That is what makes the decayed score safe to compute on
// every read instead of storing it — and it is why callers pass `now` rather
// than the package calling time.Now itself.
func Compute(in Inputs, now time.Time) Score {
	if in.LastTouch == nil {
		return Score{Bucket: BucketNone}
	}
	var s Score
	s.Recency = recency(in.LastEngaged, now)
	count := max(in.Count90d-in.LateCount90d, 0)
	outbound := max(in.Outbound90d-in.LateOutbound90d, 0)
	s.Frequency = math.Min(1.0, float64(count)/FreqSaturation)
	// Reciprocity rewards a two-way exchange and floors a one-way one. The
	// floor keeps a hundred unanswered sends from scoring as a relationship,
	// and still admits that they happened. Recency counts only from the
	// contact's own part for the same reason. A send of ours they did not
	// answer is no sign the relationship is warm.
	directed := in.Inbound90d + outbound
	balance := 0.0
	if directed > 0 {
		balance = 1 - math.Abs(float64(in.Inbound90d-outbound))/float64(directed)
	}
	s.Reciprocity = ReciprocityFloor + (1-ReciprocityFloor)*balance
	s.Strength = int(math.Round(100 * s.Recency * s.Frequency * s.Reciprocity))
	s.Bucket = bucketFor(s.Strength)
	return s
}

// recency halves every HalfLifeDays since the contact last took part. A
// contact who never took part has none: only our sends, which band weak.
func recency(lastEngaged *time.Time, now time.Time) float64 {
	if lastEngaged == nil {
		return 0
	}
	days := now.Sub(*lastEngaged).Hours() / 24
	if days < 0 {
		// A clock skew, or an interaction recorded as happening later today.
		// Clamped rather than allowed to amplify recency above 1.
		days = 0
	}
	return math.Exp2(-days / HalfLifeDays)
}

// bucketFor bands a computed score.
func bucketFor(strength int) string {
	switch {
	case strength >= strongAtLeast:
		return BucketStrong
	case strength >= moderateAtLeast:
		return BucketModerate
	default:
		return BucketWeak
	}
}
