// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Deal is what a reading needs to know about one opportunity, already resolved
// into the base currency by the caller.
//
// Deliberately not the deals module's own row type. A reading is arithmetic
// over a handful of fields, and taking the whole record would make this
// package's tests need a deal writer to say anything about rounding.
type Deal struct {
	ID    string
	Owner string
	// The deal's own money. Nil is an UNPRICED deal: real pipeline that
	// contributes nothing, and never the same thing as zero.
	AmountMinor *int64
	Currency    string
	// The same money in the snapshot's base currency, nil when no rate was
	// available. A missing rate is its own outcome and never a zero.
	BaseMinor *int64
	// The day the deal is expected to close, as a local calendar date.
	ExpectedCloseDate *time.Time
	// True when the close date is a guess nobody confirmed.
	CloseProvisional bool
	// When the deal actually closed, if it has. An instant, converted to a
	// local day before any comparison.
	ClosedAt *time.Time
	Won      bool
	Category string
	// Integer percent, the stage's own win probability.
	StageProbability int
}

// Readings are the four money answers plus the counts that say what they do not
// cover.
//
// Every money field is the SUM of per-deal integers that were each rounded
// once. No reading re-rounds a sum and no reading is derived from a different
// rounding of the same deals, which is what makes reconciliation to the cent a
// property a test can hold rather than an approximation.
type Readings struct {
	WonMinor      int64
	EvidenceMinor int64
	BestCaseMinor int64
	OpenMinor     int64
	WeightedMinor int64

	// What the money does not cover. EligibleCount minus PricedCount is the
	// gap a reader is owed: an unpriced deal is real pipeline contributing
	// zero, and presenting the total without that gap invites the reading
	// where every eligible deal was counted.
	EligibleCount      int
	PricedCount        int
	ConfirmedDateCount int
	FxMissingCount     int

	// Per-deal rows behind the sums, in the order they were given. What gets
	// frozen as contributions, and what a drill-through reads.
	Contributions []Contribution
}

// Contribution is one deal's part in one set of readings, with the membership
// decisions recorded rather than left to be recomputed.
type Contribution struct {
	DealID           string
	Owner            string
	AmountMinor      *int64
	Currency         string
	BaseMinor        *int64
	EffectiveClose   *time.Time
	CloseProvisional bool
	Category         string
	StageProbability int
	// The deal's own weighted amount, ALREADY ROUNDED. Stored beside the base
	// amount because the weighted headline is the sum of these, and a headline
	// whose parts are not persisted cannot be reconciled against anything.
	WeightedMinor int64

	InWon      bool
	InEvidence bool
	InBestCase bool
	InOpen     bool
	// Why an eligible deal contributed no money. Named rather than left to be
	// inferred from a nil amount, because "we have no price" and "we could not
	// convert it" are different facts and a reader is owed which one.
	ExclusionReason string
}

// Exclusion reasons a contribution can carry, matching the migration's CHECK.
const (
	ExcludedUnpriced  = "unpriced"
	ExcludedFxMissing = "fx_missing"
)

// The categories a rep sets on a deal, plus the one the product derives.
const (
	CategoryCommit   = "commit"
	CategoryBestCase = "best_case"
	// CategorySlipped is DERIVED, never stored: a commit or best-case deal
	// whose close date has passed, is missing, or was never confirmed. The
	// report engine spells the same rule in SQL (forecastCategoryExpr in
	// compose/report.go), because an aggregate has to categorise in the
	// database. Both answer from the same three conditions.
	CategorySlipped = "slipped"
)

// Compute answers the readings for one period over one set of deals.
//
// Pure: no clock, no database, no settings read. The period arrives resolved
// and the deals arrive converted, so what this function decides is membership
// and arithmetic — the two things worth testing exhaustively and the two that
// are impossible to test exhaustively through a store.
func Compute(period Period, asOfDay time.Time, in []Deal) (Readings, error) {
	out := Readings{Contributions: make([]Contribution, 0, len(in))}
	for _, deal := range in {
		contribution, err := contribute(period, asOfDay, deal)
		if err != nil {
			return Readings{}, err
		}
		out.EligibleCount++
		if deal.AmountMinor != nil {
			out.PricedCount++
		}
		if deal.ExpectedCloseDate != nil && !deal.CloseProvisional {
			out.ConfirmedDateCount++
		}
		if deal.AmountMinor != nil && deal.BaseMinor == nil {
			out.FxMissingCount++
		}
		base := int64(0)
		if contribution.BaseMinor != nil {
			base = *contribution.BaseMinor
		}
		// Every accumulation is checked. amount_minor is contract-unbounded, so
		// a large enough population wraps a native sum — and a wrapped headline
		// is the worst possible failure here, because it still LOOKS like a
		// number and disagrees silently with the contributions it claims to be
		// the sum of.
		for _, add := range []struct {
			into *int64
			by   int64
			when bool
		}{
			{&out.WonMinor, base, contribution.InWon},
			{&out.EvidenceMinor, base, contribution.InEvidence},
			{&out.BestCaseMinor, base, contribution.InBestCase},
			{&out.OpenMinor, base, contribution.InOpen},
			{&out.WeightedMinor, contribution.WeightedMinor, contribution.InOpen},
		} {
			if !add.when {
				continue
			}
			sum, err := addMinor(*add.into, add.by)
			if err != nil {
				return Readings{}, fmt.Errorf("forecasting: totalling deal %s: %w", deal.ID, err)
			}
			*add.into = sum
		}
		out.Contributions = append(out.Contributions, contribution)
	}
	return out, nil
}

// ErrReadingOutOfRange marks a total that cannot be represented. A refusal
// rather than a wrapped number: a headline that wrapped still looks like money
// and disagrees silently with the rows it claims to be the sum of.
var ErrReadingOutOfRange = errors.New("forecasting: a reading exceeds the representable money range")

// addMinor sums two minor-unit amounts, refusing an overflow.
func addMinor(running, add int64) (int64, error) {
	sum := running + add
	// Both operands are non-negative here (a money reading is never negative
	// pipeline, held by the table's own CHECK), so a sum smaller than either
	// operand is a wrap.
	if (add > 0 && sum < running) || (add < 0 && sum > running) {
		return 0, ErrReadingOutOfRange
	}
	return sum, nil
}

// contribute decides one deal's membership and its rounded weighted value.
func contribute(period Period, asOfDay time.Time, deal Deal) (Contribution, error) {
	out := Contribution{
		DealID:           deal.ID,
		Owner:            deal.Owner,
		AmountMinor:      deal.AmountMinor,
		Currency:         deal.Currency,
		BaseMinor:        deal.BaseMinor,
		EffectiveClose:   deal.ExpectedCloseDate,
		CloseProvisional: deal.CloseProvisional,
		Category:         EffectiveCategory(asOfDay, deal),
		StageProbability: deal.StageProbability,
	}
	switch {
	case deal.AmountMinor == nil:
		out.ExclusionReason = ExcludedUnpriced
	case deal.BaseMinor == nil:
		// Priced but unconvertible. A typed absence, never a zero: a zero here
		// would quietly shrink a total and look like a smaller pipeline rather
		// than like a missing rate.
		out.ExclusionReason = ExcludedFxMissing
	}

	// Won is decided by the day the deal ACTUALLY closed, never by the day it
	// was expected to. A deal expected in March and won in April belongs to
	// April's won reading, and counting it in March's would report money the
	// quarter did not bring in.
	out.InWon = deal.Won && deal.ClosedAt != nil && period.ContainsInstant(*deal.ClosedAt)

	if !deal.Won && deal.ExpectedCloseDate != nil && period.ContainsDay(*deal.ExpectedCloseDate) {
		out.InOpen = true
		// Evidence is the reading that claims support. A provisional date is a
		// guess, so it is in the open pipeline and out of the evidence — which
		// is what the word is for.
		out.InEvidence = !deal.CloseProvisional && out.Category == CategoryCommit
		out.InBestCase = !deal.CloseProvisional &&
			(out.Category == CategoryCommit || out.Category == CategoryBestCase)
	}

	if out.InOpen && deal.BaseMinor != nil {
		weighted, err := values.WeightedValue(*deal.BaseMinor, deal.StageProbability)
		if err != nil {
			return Contribution{}, fmt.Errorf("forecasting: weighting deal %s: %w", deal.ID, err)
		}
		out.WeightedMinor = weighted
	}
	return out, nil
}

// calendarDay drops the clock, keeping the zone the value already carries. A
// date and an instant compare as the same day or they do not compare at all.
func calendarDay(at time.Time) time.Time {
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location())
}

// EffectiveCategory answers what a deal's forecast category READS as, which is
// not always what is stored on it.
//
// A commit or best-case deal whose close date has passed, is missing, or was
// never confirmed reads as slipped. The report engine applies the same three
// conditions in SQL (forecastCategoryExpr, compose/report.go); both exist
// because an aggregate categorises in the database and this path has no
// aggregate to fold into.
//
// asOfDay is the local day the reading is taken on — the same value the SQL
// gets from timezone(<installation zone>, now())::date. Passed in rather than
// read from a clock here, so the categorisation of a set of deals is a
// function of its inputs and a test can state the day it means.
func EffectiveCategory(asOfDay time.Time, deal Deal) string {
	if deal.Category != CategoryCommit && deal.Category != CategoryBestCase {
		return deal.Category
	}
	// Against TODAY, not against the period's opening day. A deal expected
	// next week has not slipped merely because the quarter started last month,
	// and comparing to the period start would report most of a quarter's
	// healthy pipeline as slipped for the whole quarter.
	// Both sides reduced to a calendar day before comparing. asOfDay arrives
	// as an instant and a deal expected TODAY must not read as slipped from
	// noon onward — the SQL compares dates in the installation zone, and a Go
	// side comparing instants disagrees with it for exactly one day per deal,
	// on the day the deal is due.
	slipped := deal.ExpectedCloseDate == nil ||
		calendarDay(*deal.ExpectedCloseDate).Before(calendarDay(asOfDay)) ||
		deal.CloseProvisional
	if slipped {
		return CategorySlipped
	}
	return deal.Category
}
