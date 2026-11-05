// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The §3 golden tests: the spec's worked example reproduces exactly,
// the breakdown sums to the score (AC-S1), and recompute under a fixed
// clock is idempotent (AC-S2/S3).

import (
	"math"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestScoreLeadWorkedExample(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	signals := []BehavioralSignal{
		{Kind: "reply", OccurredAt: now.AddDate(0, 0, -2), ActivityID: ids.New[ids.ActivityKind]()},
		{Kind: "link_click", OccurredAt: now.AddDate(0, 0, -10), ActivityID: ids.New[ids.ActivityKind]()},
		{Kind: "link_click", OccurredAt: now.AddDate(0, 0, -10), ActivityID: ids.New[ids.ActivityKind]()},
	}
	score, factors := scoreLeadByOrigin("VP Sales", "website", signals, now)
	if score != 51 {
		t.Fatalf("worked example score = %d, want 51 (factors: %+v)", score, factors)
	}
	var sum float64
	for _, f := range factors {
		sum += f.Points
	}
	if int(math.Floor(sum+0.5)) != score {
		t.Fatalf("breakdown sums to %.2f but score is %d — Explain This Score must reconcile", sum, score)
	}
	// Idempotent under the fixed clock.
	again, _ := scoreLeadByOrigin("VP Sales", "website", signals, now)
	if again != score {
		t.Fatalf("recompute drifted: %d → %d", score, again)
	}
}

func TestScoreLeadEdges(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)

	if score, _ := scoreLeadByOrigin("Intern", "cold_outreach", nil, now); score != 0 {
		t.Errorf("negative fit must clamp at 0, got %d", score)
	}
	if score, _ := scoreLeadByOrigin("CEO", "referral", nil, now); score != 23 {
		t.Errorf("pure-fit cold lead = %d, want 23", score)
	}
	// Unknown signal kinds contribute nothing (column-readiness rule).
	score, factors := scoreLeadByOrigin("", "", []BehavioralSignal{
		{Kind: "engagement_event_not_yet_shipped", OccurredAt: now},
	}, now)
	if score != 0 || len(factors) != 0 {
		t.Errorf("unknown signal leaked into the score: %d %+v", score, factors)
	}
	// A flood of replies clamps at the max.
	var flood []BehavioralSignal
	for range 10 {
		flood = append(flood, BehavioralSignal{Kind: "reply", OccurredAt: now})
	}
	if score, _ := scoreLeadByOrigin("CTO", "website", flood, now); score != 100 {
		t.Errorf("clamp ceiling: %d, want 100", score)
	}
}

// TestScoreLeadProductionPathFixture pins what the SAME lead as the worked
// example scores through the path the product actually runs (ADR-0105 §6).
//
// The formula fixture above proves the arithmetic including link_click,
// which is the only coverage of a weight A3 ratified. It cannot also stand
// for what a rep sees, because clicks never reach the scorer: ingestion
// materializes replies and meetings only, and email_open is dead by the
// honest-signal ruling. Both numbers are true about different things, so
// both are pinned — replacing 51 with 46 would delete the click weight's
// only test until the day it starts firing.
func TestScoreLeadProductionPathFixture(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	// The worked example's lead, carrying only the signals the ingestion
	// path can produce today.
	scored := ScoreLeadDetail("VP Sales", SourceIntentHigh, []BehavioralSignal{
		{Kind: "reply", OccurredAt: now.AddDate(0, 0, -2), ActivityID: ids.New[ids.ActivityKind]()},
	}, now)
	if scored.Score != 46 {
		t.Fatalf("production-path score = %d, want 46 (factors: %+v)", scored.Score, scored.Factors)
	}
}

// TestScoreLeadDetailSeparatesRoundingFromClamping is the arithmetic the
// explain response has to be able to state. Collapsing the two steps
// reports a clamp where only rounding happened, and names a clamp source
// that never existed (ADR-0105 §2).
func TestScoreLeadDetailSeparatesRoundingFromClamping(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)

	// Rounding alone: a decayed reply lands on a fraction, well inside the
	// bounds, so rounded and stored agree and no clamp is implied.
	rounding := ScoreLeadDetail("", SourceIntentHigh, []BehavioralSignal{
		{Kind: "reply", OccurredAt: now.AddDate(0, 0, -3), ActivityID: ids.New[ids.ActivityKind]()},
	}, now)
	if rounding.RawSum == float64(rounding.RoundedSum) {
		t.Fatalf("fixture is not fractional: raw %.4f", rounding.RawSum)
	}
	if rounding.RoundedSum != rounding.Score {
		t.Errorf("no clamp should apply here: rounded %d, stored %d", rounding.RoundedSum, rounding.Score)
	}

	// Clamping: the cap acts on the ROUNDED value, so RoundedSum must
	// exceed the max while the stored score sits at it.
	var flood []BehavioralSignal
	for range 10 {
		flood = append(flood, BehavioralSignal{Kind: "reply", OccurredAt: now, ActivityID: ids.New[ids.ActivityKind]()})
	}
	clamped := ScoreLeadDetail("CTO", SourceIntentHigh, flood, now)
	if clamped.Score != leadScoreMax {
		t.Fatalf("expected the ceiling, got %d", clamped.Score)
	}
	if clamped.RoundedSum <= leadScoreMax {
		t.Errorf("rounded sum %d does not show the clamp it fired", clamped.RoundedSum)
	}
	if clamped.RawSum <= float64(leadScoreMax) {
		t.Errorf("raw sum %.2f should exceed the cap", clamped.RawSum)
	}
}

// TestBehavioralFactorsCarryTheirUndecayedBase is what lets a reader see
// the decay as arithmetic (`raw · 2^(−days/14)`, AC-leads-4/10) instead of
// being handed a number to take on faith.
func TestBehavioralFactorsCarryTheirUndecayedBase(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	scored := ScoreLeadDetail("", SourceIntentNeutral, []BehavioralSignal{
		{Kind: "reply", OccurredAt: now.AddDate(0, 0, -14), ActivityID: ids.New[ids.ActivityKind]()},
	}, now)
	if len(scored.Factors) != 1 {
		t.Fatalf("want one behavioral factor, got %+v", scored.Factors)
	}
	factor := scored.Factors[0]
	if factor.BasePoints != behavioralReplyPoints {
		t.Errorf("base points = %v, want %v", factor.BasePoints, behavioralReplyPoints)
	}
	// One half-life exactly: the decayed value is half the base, which is
	// the claim the rendered arithmetic makes.
	if math.Abs(factor.Points-float64(behavioralReplyPoints)/2) > 0.001 {
		t.Errorf("one half-life should halve the base: got %.4f from %v", factor.Points, factor.BasePoints)
	}
}

// TestWithManualFoldsHumanInputIntoOneTotal proves a rep's own input counts
// toward the same weighted score, and that rounding runs ONCE over the
// whole breakdown rather than over an already-rounded number.
func TestWithManualFoldsHumanInputIntoOneTotal(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	machine := ScoreLeadDetail("VP Sales", SourceIntentHigh, nil, now)
	withManual := machine.withManual([]ScoreFactor{{Factor: "manual:employees", Points: 8}})

	if withManual.Score != machine.Score+8 {
		t.Errorf("manual input did not count: %d → %d", machine.Score, withManual.Score)
	}
	if len(withManual.Factors) != len(machine.Factors)+1 {
		t.Errorf("manual factor is not its own row: %+v", withManual.Factors)
	}
	var sum float64
	for _, f := range withManual.Factors {
		sum += f.Points
	}
	if math.Abs(sum-withManual.RawSum) > 0.001 {
		t.Errorf("raw sum %.4f does not match its own factors (%.4f)", withManual.RawSum, sum)
	}
	// An empty input list must not disturb the run it was folded into.
	if unchanged := machine.withManual(nil); unchanged.Score != machine.Score {
		t.Errorf("empty manual set moved the score: %d → %d", machine.Score, unchanged.Score)
	}
}

// seededOriginIntents is the weighting the migration seeds. The unit scorer
// uses it as a fixture, and the integration test proves the seed and this table
// agree.
var seededOriginIntents = OriginIntents{
	"event": SourceIntentNeutral, "linkedin": SourceIntentNeutral, "other": SourceIntentNeutral,
	"referral": SourceIntentHigh, "partner": SourceIntentHigh, "website": SourceIntentHigh,
	"cold_outreach": SourceIntentLow,
}

// scoreLeadByOrigin scores a lead the way production does: the installation's
// weighting for its origin, with an empty origin meaning none recorded.
func scoreLeadByOrigin(title, origin string, signals []BehavioralSignal, now time.Time) (int, []ScoreFactor) {
	var key *string
	if origin != "" {
		key = &origin
	}
	scored := ScoreLeadDetail(title, seededOriginIntents.Of(key), signals, now)
	return scored.Score, scored.Factors
}

func TestOriginIntentsWeighOnlyAKnownOrigin(t *testing.T) {
	t.Parallel()
	intents := OriginIntents{"referral": SourceIntentHigh, "cold_outreach": SourceIntentLow}
	cases := map[string]SourceIntent{
		"referral":      SourceIntentHigh,
		"cold_outreach": SourceIntentLow,
		"retired_key":   SourceIntentNeutral,
		"import":        SourceIntentNeutral,
		"":              SourceIntentNeutral,
	}
	for origin, want := range cases {
		if got := intents.Of(&origin); got != want {
			t.Errorf("Of(%q) = %q, want %q", origin, got, want)
		}
	}
	if got := intents.Of(nil); got != SourceIntentNeutral {
		t.Errorf("Of(nil) = %q, want neutral: a lead nobody classified scores neutral", got)
	}
}

// A meeting booked ahead scores its base value, however far ahead it is.
func TestABookingAheadScoresItsBaseValue(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	for _, ahead := range []int{30, 90} {
		booking := []BehavioralSignal{{Kind: "meeting_booked", OccurredAt: now.AddDate(0, 0, ahead)}}
		if score, _ := scoreLeadByOrigin("", "", booking, now); score != behavioralMeetingBooked {
			t.Errorf("a booking %d days ahead scores %d, want its base %d", ahead, score, behavioralMeetingBooked)
		}
	}
}
