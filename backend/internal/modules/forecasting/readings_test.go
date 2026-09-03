// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"errors"
	"math"
	"math/rand"
	"testing"
	"time"
)

func testPeriod(t *testing.T) Period {
	t.Helper()
	period, err := ResolvePeriod(PeriodQuarter, time.Date(2026, time.May, 14, 12, 0, 0, 0, berlin(t)), 1, berlin(t))
	if err != nil {
		t.Fatalf("resolving the test period: %v", err)
	}
	return period
}

// day builds a local calendar day in the test zone. Every case here sits in
// one year, so the year is fixed rather than repeated at each call site.
func day(t *testing.T, m time.Month, d int) *time.Time {
	t.Helper()
	at := time.Date(testYear, m, d, 0, 0, 0, 0, berlin(t))
	return &at
}

// testYear is the year every fixture below sits in.
const testYear = 2026

func minor(v int64) *int64 { return &v }

// A deal in the period, priced, converted, confirmed and committed — the shape
// every case below varies ONE field of, so what a case proves is that field.
func healthyDeal(t *testing.T) Deal {
	t.Helper()
	return Deal{
		ID:                "d1",
		Owner:             "u1",
		AmountMinor:       minor(100_000),
		Currency:          "EUR",
		BaseMinor:         minor(100_000),
		ExpectedCloseDate: day(t, time.May, 20),
		Category:          CategoryCommit,
		StageProbability:  50,
	}
}

func TestAProvisionalDateStaysOutOfTheEvidenceReading(t *testing.T) {
	t.Parallel()
	period := testPeriod(t)
	asOf := *day(t, time.May, 14)

	confirmed, err := Compute(period, asOf, []Deal{healthyDeal(t)})
	if err != nil {
		t.Fatalf("computing: %v", err)
	}
	if confirmed.EvidenceMinor != 100_000 {
		t.Errorf("a confirmed committed deal contributed %d to evidence, want 100000", confirmed.EvidenceMinor)
	}

	guessed := healthyDeal(t)
	guessed.CloseProvisional = true
	got, err := Compute(period, asOf, []Deal{guessed})
	if err != nil {
		t.Fatalf("computing: %v", err)
	}
	if got.EvidenceMinor != 0 {
		t.Errorf("a deal whose close date nobody confirmed contributed %d to the EVIDENCE reading — "+
			"a guess is what that reading exists to exclude", got.EvidenceMinor)
	}
	// It is still real pipeline. Dropping it from open as well would understate
	// the funnel, which is a different lie from overstating the evidence.
	if got.OpenMinor != 100_000 {
		t.Errorf("a provisional deal contributed %d to open pipeline, want 100000 — it is still a live deal", got.OpenMinor)
	}
	if got.ConfirmedDateCount != 0 {
		t.Errorf("confirmed-date count was %d for a provisional date", got.ConfirmedDateCount)
	}
}

func TestAnUnpricedDealIsCountedAndContributesNothing(t *testing.T) {
	t.Parallel()
	period := testPeriod(t)
	asOf := *day(t, time.May, 14)

	unpriced := healthyDeal(t)
	unpriced.AmountMinor = nil
	unpriced.BaseMinor = nil
	unpriced.Currency = ""

	got, err := Compute(period, asOf, []Deal{unpriced})
	if err != nil {
		t.Fatalf("computing: %v", err)
	}
	if got.EligibleCount != 1 {
		t.Errorf("an unpriced deal was not counted as eligible (%d) — it is real pipeline", got.EligibleCount)
	}
	if got.PricedCount != 0 {
		t.Errorf("priced count was %d for a deal with no amount", got.PricedCount)
	}
	if got.OpenMinor != 0 || got.WeightedMinor != 0 {
		t.Errorf("an unpriced deal contributed money (open %d, weighted %d) — a missing price is not zero euros",
			got.OpenMinor, got.WeightedMinor)
	}
	if got.Contributions[0].ExclusionReason != ExcludedUnpriced {
		t.Errorf("exclusion reason was %q, want %q — the reader is owed WHY it contributed nothing",
			got.Contributions[0].ExclusionReason, ExcludedUnpriced)
	}
}

// A missing rate and a zero amount look identical in a total, and they mean
// opposite things: one is a smaller pipeline, the other is a pipeline we
// could not price.
func TestAMissingRateIsTypedRatherThanZero(t *testing.T) {
	t.Parallel()
	period := testPeriod(t)
	asOf := *day(t, time.May, 14)

	unconverted := healthyDeal(t)
	unconverted.Currency = "VND"
	unconverted.BaseMinor = nil

	got, err := Compute(period, asOf, []Deal{unconverted})
	if err != nil {
		t.Fatalf("computing: %v", err)
	}
	if got.FxMissingCount != 1 {
		t.Errorf("fx-missing count was %d, want 1", got.FxMissingCount)
	}
	if got.PricedCount != 1 {
		t.Errorf("a priced deal with no rate was counted unpriced (%d) — it HAS a price", got.PricedCount)
	}
	if got.Contributions[0].ExclusionReason != ExcludedFxMissing {
		t.Errorf("exclusion reason was %q, want %q", got.Contributions[0].ExclusionReason, ExcludedFxMissing)
	}
}

// Won is decided by the day the deal actually closed. A deal expected in this
// quarter and won in the next belongs to the next one's won reading, and
// counting it here reports money the quarter did not bring in.
func TestWonFollowsTheCloseInstantNotTheExpectedDate(t *testing.T) {
	t.Parallel()
	period := testPeriod(t)
	asOf := *day(t, time.May, 14)

	closedInside := healthyDeal(t)
	closedInside.Won = true
	closedInside.ClosedAt = day(t, time.May, 30)

	got, err := Compute(period, asOf, []Deal{closedInside})
	if err != nil {
		t.Fatalf("computing: %v", err)
	}
	if got.WonMinor != 100_000 {
		t.Errorf("a deal closed inside the period contributed %d to won, want 100000", got.WonMinor)
	}
	// A won deal is no longer open pipeline: counting it in both would report
	// the same money twice in one answer.
	if got.OpenMinor != 0 {
		t.Errorf("a won deal contributed %d to open pipeline — that is the same money counted twice", got.OpenMinor)
	}

	closedAfter := healthyDeal(t)
	closedAfter.Won = true
	closedAfter.ClosedAt = day(t, time.July, 2)
	after, err := Compute(period, asOf, []Deal{closedAfter})
	if err != nil {
		t.Fatalf("computing: %v", err)
	}
	if after.WonMinor != 0 {
		t.Errorf("a deal closed AFTER the period contributed %d to its won reading", after.WonMinor)
	}
}

func TestADealOutsideThePeriodIsInNoReading(t *testing.T) {
	t.Parallel()
	period := testPeriod(t)
	asOf := *day(t, time.May, 14)

	elsewhere := healthyDeal(t)
	elsewhere.ExpectedCloseDate = day(t, time.September, 3)

	got, err := Compute(period, asOf, []Deal{elsewhere})
	if err != nil {
		t.Fatalf("computing: %v", err)
	}
	if got.OpenMinor != 0 || got.EvidenceMinor != 0 || got.BestCaseMinor != 0 || got.WonMinor != 0 {
		t.Errorf("a deal expected outside the period reached a reading: open %d evidence %d best %d won %d",
			got.OpenMinor, got.EvidenceMinor, got.BestCaseMinor, got.WonMinor)
	}
}

func TestSlippedIsJudgedAgainstTodayNotThePeriodStart(t *testing.T) {
	t.Parallel()

	// Expected next week, in a quarter that opened six weeks ago. Healthy —
	// and the case a comparison against the PERIOD start would wrongly call
	// slipped for most of every quarter.
	upcoming := healthyDeal(t)
	upcoming.ExpectedCloseDate = day(t, time.May, 20)
	if got := EffectiveCategory(*day(t, time.May, 14), upcoming); got != CategoryCommit {
		t.Errorf("a deal expected next week reads as %q — it has not slipped", got)
	}

	// The same deal, read a month later, after its date went by.
	if got := EffectiveCategory(*day(t, time.June, 14), upcoming); got != CategorySlipped {
		t.Errorf("a deal whose date has passed reads as %q, want %q", got, CategorySlipped)
	}
}

// The rounding contract, as a property.
//
// Every headline must equal the sum of the stored per-deal integers EXACTLY.
// Asserted against the stored contributions rather than a re-derivation,
// because per-deal rounding and sum-then-round differ by up to one minor unit
// per deal — which is the whole reason the contract exists.
func TestEveryHeadlineIsTheSumOfItsStoredContributions(t *testing.T) {
	t.Parallel()
	period := testPeriod(t)
	asOf := *day(t, time.May, 14)
	rng := rand.New(rand.NewSource(20260903))

	for run := range 200 {
		population := make([]Deal, 0, 12)
		for i := range rng.Intn(12) + 1 {
			deal := healthyDeal(t)
			deal.ID = string(rune('a' + i))
			// Amounts spanning the minor-unit scales the product supports:
			// JPY counts whole yen, BHD counts thousandths.
			amount := int64(rng.Intn(9_000_000)) - 1_000_000
			if amount < 0 {
				amount = -amount
			}
			deal.AmountMinor = minor(amount)
			deal.BaseMinor = minor(amount)
			deal.StageProbability = rng.Intn(101)
			switch rng.Intn(4) {
			case 0:
				deal.Category = CategoryBestCase
			case 1:
				deal.CloseProvisional = true
			case 2:
				deal.Won = true
				deal.ClosedAt = day(t, time.May, 30)
			}
			population = append(population, deal)
		}

		got, err := Compute(period, asOf, population)
		if err != nil {
			t.Fatalf("run %d: computing: %v", run, err)
		}

		var won, evidence, best, open, weighted int64
		for _, c := range got.Contributions {
			base := int64(0)
			if c.BaseMinor != nil {
				base = *c.BaseMinor
			}
			if c.InWon {
				won += base
			}
			if c.InEvidence {
				evidence += base
			}
			if c.InBestCase {
				best += base
			}
			if c.InOpen {
				open += base
				weighted += c.WeightedMinor
			}
		}
		for _, check := range []struct {
			name             string
			headline, summed int64
		}{
			{"won", got.WonMinor, won},
			{"evidence", got.EvidenceMinor, evidence},
			{"best case", got.BestCaseMinor, best},
			{"open", got.OpenMinor, open},
			{"weighted", got.WeightedMinor, weighted},
		} {
			if check.headline != check.summed {
				t.Fatalf("run %d: the %s headline is %d and its stored contributions sum to %d — "+
					"a drill-through would not add up to the number above it",
					run, check.name, check.headline, check.summed)
			}
		}
	}
}

// A deal due TODAY has not slipped, and the Go side must agree with the SQL
// about that. The SQL compares calendar dates in the installation zone; a Go
// side comparing instants calls the same deal slipped from noon onward, so the
// forecast screen and the report engine disagree for one day per deal — on the
// day the deal is actually due, which is the day anyone is looking at it.
func TestADealDueTodayHasNotSlippedAtAnyHour(t *testing.T) {
	t.Parallel()
	zone := berlin(t)
	due := healthyDeal(t)
	due.ExpectedCloseDate = day(t, time.May, 14)

	for _, hour := range []int{0, 12, 23} {
		asOf := time.Date(testYear, time.May, 14, hour, 30, 0, 0, zone)
		if got := EffectiveCategory(asOf, due); got != CategoryCommit {
			t.Errorf("read at %02d:30 on the day it is due, the deal reads as %q — "+
				"the report engine says %q at every hour of that day",
				hour, got, CategoryCommit)
		}
	}
	// The admitting case: the day after, it really has slipped.
	if got := EffectiveCategory(time.Date(testYear, time.May, 15, 0, 30, 0, 0, zone), due); got != CategorySlipped {
		t.Errorf("the day after it was due, the deal reads as %q, want %q", got, CategorySlipped)
	}
}

// A headline that wrapped still looks like money. It would disagree silently
// with the contributions it claims to be the sum of, which is the one property
// this whole module promises.
func TestAHeadlineRefusesToWrapRatherThanReportANumber(t *testing.T) {
	t.Parallel()
	period := testPeriod(t)
	asOf := *day(t, time.May, 14)

	huge := int64(math.MaxInt64)
	population := make([]Deal, 3)
	for i := range population {
		deal := healthyDeal(t)
		deal.ID = string(rune('a' + i))
		deal.AmountMinor = &huge
		deal.BaseMinor = &huge
		deal.Won = true
		deal.ClosedAt = day(t, time.May, 30)
		population[i] = deal
	}

	if _, err := Compute(period, asOf, population); !errors.Is(err, ErrReadingOutOfRange) {
		t.Errorf("three maximal won deals totalled to err=%v — a sum that cannot be "+
			"represented must refuse, never wrap into a plausible-looking figure", err)
	}

	// The admitting case. Without it, an accumulator that refused EVERY total
	// would pass the assertion above.
	if _, err := Compute(period, asOf, []Deal{healthyDeal(t)}); err != nil {
		t.Errorf("an ordinary population was refused: %v", err)
	}
}

// A Period's two spellings are read by different code paths — the date half by
// the expected-close comparison, the instant half by the close-instant one. A
// hand-assembled Period whose halves disagree puts a deal in one reading and
// not the other.
func TestAPeriodWhoseHalvesDisagreeIsNotConsistent(t *testing.T) {
	t.Parallel()
	zone := berlin(t)
	good, err := ResolvePeriod(PeriodQuarter, time.Date(testYear, time.May, 14, 12, 0, 0, 0, zone), 1, zone)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if !good.consistent() {
		t.Error("a period built by ResolvePeriod reads as inconsistent — the builder and the check disagree")
	}

	forked := good
	forked.EndDate = forked.EndDate.AddDate(0, 1, 0)
	if forked.consistent() {
		t.Error("a period whose day bounds name a different window than its instant bounds passed the check")
	}

	zoneless := good
	zoneless.Zone = nil
	if zoneless.consistent() {
		t.Error("a period with no zone passed the check — which day an instant falls on has no answer without one")
	}
}
