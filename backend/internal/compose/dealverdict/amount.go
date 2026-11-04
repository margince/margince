// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// decideAmount checks a proposed amount against the quote, the accepted offer
// and the rates, and sets the change in the deal's own currency.
func decideAmount(in Input, state State, action Action, rates Rates, change *deals.VerdictChange) (string, error) {
	if !quoteFound(in, action.Cited, action.QuotedWords) {
		return DroppedQuoteNotFound, nil
	}
	whole, currency, ok := quotedPrice(action.QuotedWords)
	if !ok || whole != action.Amount || currency != action.Currency {
		return DroppedAmountMismatch, nil
	}
	quotedMinor, ok := toMinor(action.Amount, values.MinorUnitScale(action.Currency))
	if !ok {
		return DroppedAmountMismatch, nil
	}
	change.Evidence["quoted_words"] = action.QuotedWords
	change.Evidence["quoted_amount"] = values.MajorUnits(quotedMinor, action.Currency)
	change.Evidence["quoted_currency"] = action.Currency
	target := action.Currency
	if state.Seen.Currency != nil {
		target = *state.Seen.Currency
	} else {
		change.Currency = action.Currency
	}
	if offer := state.AcceptedOffer; offer != nil {
		if action.Currency != offer.Currency || quotedMinor != offer.GrossMinor || target != offer.Currency {
			return DroppedOfferFixes, nil
		}
		return setAmount(state, change, offer.GrossMinor), nil
	}
	if target == action.Currency {
		return setAmount(state, change, quotedMinor), nil
	}
	converted, ok, err := convert(action.Amount, action.Currency, target, rates, change.Evidence)
	if err != nil || !ok {
		return DroppedNoRate, err
	}
	return setAmount(state, change, converted), nil
}

func setAmount(state State, change *deals.VerdictChange, amountMinor int64) string {
	if state.Seen.AmountMinor != nil && *state.Seen.AmountMinor == amountMinor && change.Currency == "" {
		return DroppedUnchanged
	}
	change.AmountMinor = amountMinor
	return ""
}

// convert takes whole units of one currency into the deal's currency through
// the base currency: amount × rate(from→base) ÷ rate(to→base), rounded half
// away from zero to whole units. The rates and their dates go in evidence.
func convert(amount int64, from, to string, rates Rates, evidence map[string]any) (int64, bool, error) {
	fromRate, ok, err := rates.For(from)
	if err != nil || !ok {
		return 0, false, err
	}
	toRate, ok, err := rates.For(to)
	if err != nil || !ok || !finite(fromRate.Rate) || !finite(toRate.Rate) || toRate.Rate.Int.Sign() <= 0 {
		return 0, false, err
	}
	numerator := new(big.Int).Mul(big.NewInt(amount), fromRate.Rate.Int)
	denominator := new(big.Int).Set(toRate.Rate.Int)
	scale(numerator, denominator, fromRate.Rate.Exp)
	scale(denominator, numerator, toRate.Rate.Exp)
	whole := deals.DivRoundHalfAwayFromZero(numerator, denominator)
	if !whole.IsInt64() {
		return 0, false, nil
	}
	minor, ok := toMinor(whole.Int64(), values.MinorUnitScale(to))
	if !ok {
		return 0, false, nil
	}
	evidence["rate_from"] = rateText(fromRate)
	evidence["rate_to"] = rateText(toRate)
	evidence["rate_date"] = rateDate(fromRate, toRate).Format(time.DateOnly)
	return minor, true, nil
}

// scale multiplies by 10^exp: into side when exp is positive, into other when
// it is negative, so the fraction stays exact.
func scale(side, other *big.Int, exp int32) {
	if exp >= 0 {
		side.Mul(side, deals.Pow10(int64(exp)))
		return
	}
	other.Mul(other, deals.Pow10(int64(-exp)))
}

func finite(rate pgtype.Numeric) bool {
	return rate.Valid && !rate.NaN && rate.InfinityModifier == pgtype.Finite
}

func rateText(rate deals.FXRate) string {
	value, err := rate.Rate.Value()
	text, ok := value.(string)
	if err != nil || !ok {
		return ""
	}
	return text
}

// rateDate is the older of the two rates' dates: the conversion is as old as
// its oldest rate.
func rateDate(a, b deals.FXRate) time.Time {
	if a.On.Before(b.On) {
		return a.On
	}
	return b.On
}
