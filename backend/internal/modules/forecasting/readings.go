// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	Name  string
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
	// Lost deals cannot land, so no reading counts them as open.
	Lost bool
	// WinProbability is the deal's own probability where one is set, else its
	// stage's typed one, in percent. It decides Likely; Source says which.
	WinProbability int
	Source         ProbabilitySource
	// StageProbability is the chance in use, an integer percent. It is the
	// deal's own probability where set, else the typed stage chance moved by
	// the stage's recorded outcomes when there are any.
	StageProbability int
	// The typed chance, the count of outcomes that moved it, the risk signals
	// and the strongest one's odds ratio. Nil where the reader set none.
	TypedProbability *int
	EvidenceN        *int
	Signals          []Signal
	Effect           *float64
	// StageID is which stage the deal stands in, empty where the reader could
	// not name one. Frozen onto the contribution so a later conversion read can
	// ask what happened to deals that REACHED a stage — a question the deal's
	// current stage cannot answer, because it has moved since.
	StageID string
}

// Readings are the four money answers plus the counts that say what they do not
// cover.
//
// Every money field is the SUM of per-deal integers that were each rounded
// once. No reading re-rounds a sum and no reading is derived from a different
// rounding of the same deals, which is what makes reconciliation to the cent a
// property a test can hold rather than an approximation.
type Readings struct {
	WonMinor int64
	// EvidenceMinor is the Likely reading: open deals at or above
	// LikelyThreshold with a confirmed close date, counted in full.
	EvidenceMinor int64
	OpenMinor     int64
	WeightedMinor int64
	// LikelyThreshold is the probability this read counted Likely from.
	LikelyThreshold int
	// BySource splits the open and weighted readings by probability source;
	// each adds up to its reading.
	BySource map[ProbabilitySource]SourceTotal

	// What the money does not cover, in TWO gaps rather than one. An unpriced
	// deal has no amount to contribute; a priced deal no rate could reach has
	// one and still contributes nothing, so it sits inside PricedCount and
	// outside every total. What the money covers is PricedCount minus
	// FxMissingCount, and presenting a total without that figure invites the
	// reading where every eligible deal was counted. CoverageNote says it.
	EligibleCount      int
	PricedCount        int
	ConfirmedDateCount int
	FxMissingCount     int

	// Per-deal rows behind the sums, in the order they were given. What gets
	// frozen as contributions, and what a drill-through reads.
	Contributions []Contribution

	// Range is the central 80% of finishes, set by the assembler; nil if not drawn.
	Range *Range
}

// Contribution is one deal's part in one set of readings, with the membership
// decisions recorded rather than left to be recomputed.
type Contribution struct {
	DealID string
	// Name is the deal's name as read live, for a drill-through. Never frozen.
	Name             string
	Owner            string
	AmountMinor      *int64
	Currency         string
	BaseMinor        *int64
	EffectiveClose   *time.Time
	CloseProvisional bool
	// Source is where the deal's probability came from; empty on a row frozen
	// before it was recorded.
	Source           ProbabilitySource
	StageProbability int
	// What the chance and the risk effect were made of; see Deal. Nil on a
	// contribution frozen before they were recorded.
	TypedProbability *int
	EvidenceN        *int
	Signals          []Signal
	Effect           *float64
	// StageID is the stage the deal stood in when this was frozen, empty where
	// it could not be read. Carried so a conversion rate can be measured per
	// stage from what was recorded rather than from where the deal ended up.
	StageID string
	// The deal's own weighted amount, ALREADY ROUNDED. Stored beside the base
	// amount because the weighted headline is the sum of these, and a headline
	// whose parts are not persisted cannot be reconciled against anything.
	WeightedMinor int64

	InWon bool
	// InEvidence is membership of the Likely reading.
	InEvidence bool
	InOpen     bool
	// InLost marks a deal that closed lost: in no reading, kept so a movement
	// can name the loss instead of calling it pushed out.
	InLost bool
	// Why an eligible deal contributed no money. Named rather than left to be
	// inferred from a nil amount, because "we have no price" and "we could not
	// convert it" are different facts and a reader is owed which one.
	ExclusionReason string

	// What changed this deal since the previous snapshot, recorded at write
	// time. audit_log has no correlation column to join on later, so
	// reconstructing the link at read time means guessing from timestamps —
	// wrong exactly when two changes land in the same second, which a busy
	// pipeline produces daily. Nil means the deal did not change, which is a
	// real answer and the common one.
	AuditID *ids.UUID
	// The approval that authorised the change, where one did. Naming the actor
	// without naming what let them act tells half the story.
	ApprovalID *ids.UUID
}

// Exclusion reasons a contribution can carry, matching the migration's CHECK.
const (
	ExcludedUnpriced  = "unpriced"
	ExcludedFxMissing = "fx_missing"
)

// ProbabilitySource is where a deal's win probability comes from, spelled as
// the deal module's deal_win_probability_set_by column and forecast
// contribution's probability_source spell it.
type ProbabilitySource string

// The three sources a probability comes from.
const (
	SourceStage    ProbabilitySource = "stage"
	SourceHuman    ProbabilitySource = "human"
	SourceMargince ProbabilitySource = "margince"
)

// ProbabilitySources lists the sources in the order a reader is shown them.
func ProbabilitySources() []ProbabilitySource {
	return []ProbabilitySource{SourceStage, SourceHuman, SourceMargince}
}

// SourceTotal is the open and weighted money of the deals one source gave a
// probability to.
type SourceTotal struct {
	OpenMinor     int64
	WeightedMinor int64
}

// Rules are the installation's settings a read applies.
type Rules struct {
	// LikelyThreshold is the probability, in percent, from which an open deal
	// with a confirmed close date counts in the Likely reading.
	LikelyThreshold int
}

// Compute answers the readings for one period over one set of deals.
//
// Pure: no clock, no database, no settings read. The period arrives resolved
// and the deals arrive converted, so what this function decides is membership
// and arithmetic — the two things worth testing exhaustively and the two that
// are impossible to test exhaustively through a store.
func Compute(period Period, in []Deal, rules Rules) (Readings, error) {
	// The rows come in no stated order. Evidence pages are cut by position, so
	// the contributions follow the deal id and a next page repeats no row.
	byID := slices.SortedFunc(slices.Values(in), func(a, b Deal) int { return strings.Compare(a.ID, b.ID) })
	out := Readings{
		Contributions: make([]Contribution, 0, len(in)), LikelyThreshold: rules.LikelyThreshold,
		BySource: map[ProbabilitySource]SourceTotal{},
	}
	for _, source := range ProbabilitySources() {
		out.BySource[source] = SourceTotal{}
	}
	for _, deal := range byID {
		contribution, err := contribute(period, deal, rules)
		if err != nil {
			return Readings{}, err
		}
		// A lost deal is in no reading, so no coverage count includes it; its
		// contribution row is still kept for the snapshot.
		if !deal.Lost {
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
		if err := addBySource(&out, contribution, base); err != nil {
			return Readings{}, fmt.Errorf("forecasting: totalling deal %s: %w", deal.ID, err)
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

// addBySource adds an open deal's money to its source's totals, with the same
// overflow refusal as every other total.
func addBySource(out *Readings, c Contribution, base int64) error {
	if !c.InOpen {
		return nil
	}
	source := c.Source
	if source == "" {
		source = SourceStage
	}
	total := out.BySource[source]
	var err error
	if total.OpenMinor, err = addMinor(total.OpenMinor, base); err != nil {
		return err
	}
	if total.WeightedMinor, err = addMinor(total.WeightedMinor, c.WeightedMinor); err != nil {
		return err
	}
	out.BySource[source] = total
	return nil
}

// contribute decides one deal's membership and its rounded weighted value.
func contribute(period Period, deal Deal, rules Rules) (Contribution, error) {
	out := Contribution{
		DealID:           deal.ID,
		Name:             deal.Name,
		Owner:            deal.Owner,
		AmountMinor:      deal.AmountMinor,
		Currency:         deal.Currency,
		BaseMinor:        deal.BaseMinor,
		EffectiveClose:   deal.ExpectedCloseDate,
		CloseProvisional: deal.CloseProvisional,
		Source:           deal.Source,
		StageProbability: deal.StageProbability,
		TypedProbability: deal.TypedProbability,
		EvidenceN:        deal.EvidenceN,
		Signals:          deal.Signals,
		Effect:           deal.Effect,
		StageID:          deal.StageID,
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

	out.InLost = deal.Lost

	if !deal.Won && !deal.Lost && deal.ExpectedCloseDate != nil && period.ContainsDay(*deal.ExpectedCloseDate) {
		out.InOpen = true
		// Likely is the reading that claims support. A provisional date is a
		// guess, so it is in the open pipeline and out of Likely.
		out.InEvidence = !deal.CloseProvisional && deal.WinProbability >= rules.LikelyThreshold
	}

	if out.InOpen && deal.BaseMinor != nil {
		weighted, err := values.WeightedValue(*deal.BaseMinor, AppliedProbability(deal.StageProbability, deal.Effect))
		if err != nil {
			return Contribution{}, fmt.Errorf("forecasting: weighting deal %s: %w", deal.ID, err)
		}
		out.WeightedMinor = weighted
	}
	return out, nil
}

// CoverageNote states, in one line, what the money readings do not cover —
// empty when they cover everything.
//
// Written from the counts and adding no fact a caller could not compute, the
// way ContactMoment.headline is the reason in one line written from the
// evidence. What it buys is that the comparison is MADE: the counts arrive as
// bare integers among nine, and a total quoted flat is arithmetically correct
// while telling somebody something false about how much pipeline it covers.
//
// COVERED is priced minus unconvertible. A deal with an amount no rate could
// reach carries a price and contributes nothing, so leading with the priced
// count would understate the gap — the one direction this sentence exists to
// prevent.
//
// FOR THE MODEL-FACING DOOR, and deliberately not the contract. The web app
// states the same fact from the same counts in the reader's own language
// (`forecast.partial`), and an English sentence on the wire would be a second
// spelling that no localised client renders. A model has no i18n layer, which
// is what makes prose the right shape there and the wrong shape here.
func (r Readings) CoverageNote() string {
	covered := r.PricedCount - r.FxMissingCount
	var parts []string
	if covered < r.EligibleCount {
		parts = append(parts, fmt.Sprintf(
			"the money readings cover %d of %d eligible deals", covered, r.EligibleCount))
	}
	if unpriced := r.EligibleCount - r.PricedCount; unpriced > 0 {
		parts = append(parts, fmt.Sprintf("%s %s no amount and %s zero",
			deals(unpriced), plural(unpriced, "carries", "carry"),
			plural(unpriced, "contributes", "contribute")))
	}
	if r.FxMissingCount > 0 {
		parts = append(parts, fmt.Sprintf(
			"%s %s priced in a currency no rate could convert, so %s absent from the totals "+
				"rather than counted as zero",
			deals(r.FxMissingCount), plural(r.FxMissingCount, "was", "were"),
			plural(r.FxMissingCount, "it is", "they are")))
	}
	return strings.Join(parts, "; ")
}

// deals renders the count and its noun, so a sentence names its own subject.
func deals(n int) string {
	if n == 1 {
		return "1 deal"
	}
	return fmt.Sprintf("%d deals", n)
}

// plural picks the form the count agrees with.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
