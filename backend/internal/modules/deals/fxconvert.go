// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Converting one money amount to the installation's base currency, at the rate
// the estate stored.
//
// It lives HERE because this module owns fx_rate, and it is exported because
// two readers outside the module ask the same question of it: the hierarchy
// rollup's weighted pipeline, and the company page's own open-pipeline read.
// They had one implementation each — Go over big.Int in one, a SQL lateral with
// Postgres round() in the other — encoding the same five decisions: the
// direction, the as-of cutoff, newest-wins, the multiply-and-round, and the
// minor-unit scales the two sides count in.
//
// The two agreed. Nothing made them keep agreeing, and the first divergence
// would have been rounding — half-away-from-zero against Postgres round() — so
// a one-minor-unit disagreement between the company page and the rollup for the
// same account, which is the class of defect nobody can reproduce on demand.
//
// WHAT IS NOT SHARED is the missing-rate policy, and that is deliberate: the
// rollup refuses the whole read, because a partial total presented as a total
// is a lie about money; the company page prices what it can and counts the rest,
// because its contract says how many deals reached the figure. Both are right
// for their own surface. The engine answers "is there a rate, and what does it
// make of this amount" and leaves what to DO about a missing one to the caller.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// FXRate is one stored rate and the day it is dated. They are ONE answer and
// must travel together: a date coalesced from somewhere else could name a day
// whose rate is not the rate a figure was computed at.
type FXRate struct {
	Rate pgtype.Numeric
	On   time.Time
}

// FXRates answers what one currency converts at, memoizing a lookup per
// currency for the life of one read.
//
// Per READ and not per process: a rate is appended forward and a long-lived
// cache would price a rollup at a rate the estate has since corrected. The
// caller builds one, uses it, and drops it.
type FXRates struct {
	base  string
	asOf  time.Time
	found map[string]FXRate
	// missing is remembered too. A currency with no rate is a fact about the
	// estate, and re-asking per deal turns one refusal into a query per row.
	missing map[string]bool
}

// NewFXRates opens a lookup against the installation's base currency, reading
// the newest rate dated on or before asOf.
func NewFXRates(base string, asOf time.Time) *FXRates {
	return &FXRates{base: base, asOf: asOf, found: map[string]FXRate{}, missing: map[string]bool{}}
}

// Base is the currency everything converts to.
func (r *FXRates) Base() string { return r.base }

// For answers the rate that converts currency into the base, and whether the
// estate holds one at all.
//
// A currency that IS the base answers a rate of exactly 1 dated the as-of day,
// rather than making every caller special-case it. That is not an invented
// rate: it is the identity, and the alternative — each caller comparing
// currencies before asking — is how one of them comes to compare them
// differently.
func (r *FXRates) For(ctx context.Context, tx pgx.Tx, currency string) (FXRate, bool, error) {
	if currency == r.base {
		return FXRate{Rate: oneRate(), On: r.asOf}, true, nil
	}
	if rate, ok := r.found[currency]; ok {
		return rate, true, nil
	}
	if r.missing[currency] {
		return FXRate{}, false, nil
	}
	var rate FXRate
	// The as-of day is the UTC calendar date, matching fx_rate's
	// one-rate-per-pair-per-UTC-day grain; the text bind + cast keeps the
	// comparison independent of the session timezone.
	err := tx.QueryRow(ctx, `
		SELECT rate, rate_date FROM fx_rate
		WHERE from_currency = $1 AND to_currency = $2 AND rate_date <= $3::date
		ORDER BY rate_date DESC
		LIMIT 1`,
		currency, r.base, r.asOf.Format(time.DateOnly)).Scan(&rate.Rate, &rate.On)
	if errors.Is(err, pgx.ErrNoRows) {
		r.missing[currency] = true
		return FXRate{}, false, nil
	}
	if err != nil {
		return FXRate{}, false, fmt.Errorf("read the stored FX rate for %s: %w", currency, err)
	}
	r.found[currency] = rate
	return rate, true, nil
}

// oneRate is the identity rate, as a numeric rather than as a special case in
// the arithmetic below.
func oneRate() pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(1), Exp: 0, Valid: true}
}

// The two ways a conversion refuses, as sentinels rather than as one opaque
// error, because a caller must be able to tell them apart and one of them
// silently was not.
//
// ErrAmountOutOfRange is about ONE amount: this deal's converted value does not
// fit money's range. A surface reporting a partial figure skips that row and
// counts it.
//
// ErrRateNotFinite is about the ESTATE: a stored fx_rate row holds NaN or an
// infinity, so every amount in that currency is unconvertible and will stay so
// until an operator corrects the row. Skipping it row by row would report a
// short total as a whole one, quietly, for as long as the bad row survives —
// which is the shape of defect this whole file exists to remove.
var (
	ErrRateNotFinite    = errors.New("stored FX rate is not a finite number; correct the fx_rate row before retrying")
	ErrAmountOutOfRange = errors.New("converted amount exceeds the representable money range in the base currency")
)

// PricedAmount is what one amount came to in the base currency, and at which
// day's rate. Both or neither: an amount the estate cannot price answers
// Priced false, and the two fields together are the only honest way to state a
// converted figure — a date coalesced from elsewhere could name a day whose
// rate is not the rate this figure was computed at.
type PricedAmount struct {
	Minor  int64
	RateOn time.Time
	Priced bool
}

// PriceAll converts a batch of amounts to the base currency, leaving the ones
// the estate cannot price UNPRICED rather than refusing the whole batch.
//
// This is the loop every open-money surface runs — the Worklist's ranking, the
// company page's open pipeline — spelled once beside the engine it drives, so
// the four decisions inside it (which rate, what a missing one means, what an
// unrepresentable product means, and the two minor-unit scales) cannot come
// apart between callers. It was two copies before, agreeing by inspection, and
// they stopped agreeing on the fourth.
//
// Held by: TestOnlyOnePlaceMultipliesAnAmountByAStoredRate
// (backend/gates/fxconversioncallers_test.go), which fails when a second Go
// site multiplies an amount by a stored rate itself. The SQL spellings are
// outside what that test can read; the gate's own comment says which tests
// hold those.
//
// A caller wanting the OTHER missing-rate policy — refuse the read rather than
// report a partial figure, which is what the hierarchy rollup owes its readers
// — asks per amount through FXRates.For instead. That difference is the one
// thing this deliberately does not decide; the file header says why.
func PriceAll(
	ctx context.Context, tx pgx.Tx, rates *FXRates, amounts []CurrencyAmount,
) ([]PricedAmount, error) {
	out := make([]PricedAmount, len(amounts))
	for i, amount := range amounts {
		rate, found, err := rates.For(ctx, tx, amount.Currency)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		converted, err := ConvertToBase(amount.Minor, rate.Rate, amount.Currency, rates.Base())
		switch {
		case errors.Is(err, ErrAmountOutOfRange):
			// UNPRICED, not refused — a decision rather than a swallow. An
			// amount whose converted value does not fit money's range is one
			// row the surface cannot state, and a caller reports that with its
			// own count of what the figure covers. One implausible amount must
			// not take a whole page offline.
			continue
		case err != nil:
			// Anything else is a fault about the ESTATE rather than about this
			// amount — a corrupt stored rate makes every amount in that
			// currency unconvertible — so it travels to the caller instead of
			// being counted as one unpriced row.
			return nil, err
		}
		out[i] = PricedAmount{Minor: converted, RateOn: rate.On, Priced: true}
	}
	return out, nil
}

// CurrencyAmount is one figure in the currency its own record carries — the
// input side of a conversion, before it is comparable to anything.
type CurrencyAmount struct {
	Minor    int64
	Currency string
}

// ConvertToBase converts an amount of `from`'s minor units into `base`'s,
// rounding half away from zero in EXACT decimal arithmetic over the rate's
// stored numeric digits (Int × 10^Exp).
//
// Never float64, so a converted amount carries the same exactness Postgres
// ROUND over numeric gives a closed deal's frozen figure, and an amount past
// float64's 2^53 exact-integer ceiling cannot lose a minor unit.
//
// THE MINOR-UNIT SCALES MUST BOTH BE APPLIED, and this is the whole reason the
// function takes two currency codes rather than a bare amount. A stored rate
// says what ONE MAJOR UNIT is worth — the ingestion prompt reads "1 <from> =
// <rate> <to>" off a rates page — while the amounts on both sides are counts
// of MINOR units. Those scales differ: JPY has no minor unit and EUR has two,
// so ¥5,000,000 (five million yen) × 0.006 gives 30,000, which is read as
// €300.00 and is a hundredth of the €30,000 the deal is worth. Multiplying a
// minor-unit amount by a major-unit rate is only correct while both currencies
// happen to carry two digits, which every pair in the demo data does.
//
// So the amount is taken up to major units by `from`'s scale and back down by
// `base`'s, as one exact fraction — never as two roundings, which would lose a
// minor unit on the way through.
//
// A non-finite rate or an overflowing result refuses loudly: both would
// otherwise put a silently wrong number in a money total.
func ConvertToBase(amountMinor int64, rate pgtype.Numeric, from, base string) (int64, error) {
	if !rate.Valid || rate.NaN || rate.InfinityModifier != pgtype.Finite {
		return 0, ErrRateNotFinite
	}
	// numerator/denominator is amountMinor × rate × 10^base ÷ 10^from, kept as
	// one fraction so the single rounding at the end is the only one.
	numerator := new(big.Int).Mul(big.NewInt(amountMinor), rate.Int)
	denominator := big.NewInt(1)
	if rate.Exp >= 0 {
		numerator.Mul(numerator, Pow10(int64(rate.Exp)))
	} else {
		denominator.Mul(denominator, Pow10(int64(-rate.Exp)))
	}
	numerator.Mul(numerator, Pow10(int64(values.MinorUnitDigits(base))))
	denominator.Mul(denominator, Pow10(int64(values.MinorUnitDigits(from))))
	product := DivRoundHalfAwayFromZero(numerator, denominator)
	if !product.IsInt64() {
		return 0, ErrAmountOutOfRange
	}
	return product.Int64(), nil
}

// DivRoundHalfAwayFromZero is numerator/denominator with the quotient rounded
// half away from zero. denominator is always a positive power of ten here.
func DivRoundHalfAwayFromZero(numerator, denominator *big.Int) *big.Int {
	negative := numerator.Sign() < 0
	quotient, remainder := new(big.Int).QuoRem(numerator, denominator, new(big.Int))
	remainder.Abs(remainder)
	remainder.Lsh(remainder, 1) // 2·|remainder| ≥ denominator ⇔ the dropped fraction is ≥ half
	if remainder.Cmp(denominator) < 0 {
		return quotient
	}
	if negative {
		return quotient.Sub(quotient, big.NewInt(1))
	}
	return quotient.Add(quotient, big.NewInt(1))
}

// Pow10 returns 10^exp as a big integer; exp is a numeric's scale magnitude,
// always small and never negative here.
func Pow10(exp int64) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil)
}
