// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"math/big"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// decideAmount checks a proposed amount against the quote, the accepted offer
// and the rates, and sets the change in the deal's own currency. The amount
// written is the one the quote states, to the minor unit.
func decideAmount(in Input, state State, action Action, rates Rates, change *deals.VerdictChange) (string, error) {
	if !quoteFound(in, action.Cited, action.QuotedWords) {
		return DroppedQuoteNotFound, nil
	}
	quotedMinor, currency, ok := quotedPrice(action.QuotedWords)
	if !ok || currency != action.Currency || !nearQuote(action.Amount, quotedMinor, currency) {
		return DroppedAmountMismatch, nil
	}
	change.Evidence["quoted_words"] = action.QuotedWords
	change.Evidence["quoted_amount"] = values.MajorUnits(quotedMinor, currency)
	change.Evidence["quoted_currency"] = currency
	target := currency
	if state.Seen.Currency != nil {
		target = *state.Seen.Currency
	} else {
		change.Currency = currency
	}
	change.Evidence["amount_currency"] = target
	if offer := state.AcceptedOffer; offer != nil {
		if currency != offer.Currency || quotedMinor != offer.GrossMinor || target != offer.Currency {
			return DroppedOfferFixes, nil
		}
		return setAmount(state, change, offer.GrossMinor), nil
	}
	if target == currency {
		return setAmount(state, change, quotedMinor), nil
	}
	converted, ok, err := convert(quotedMinor, currency, target, rates, change.Evidence)
	if err != nil || !ok {
		return DroppedNoRate, err
	}
	return setAmount(state, change, converted), nil
}

// nearQuote reports whether the model's whole-unit amount is the quoted one,
// within the one unit a whole number cannot state.
func nearQuote(whole, quotedMinor int64, currency string) bool {
	minor, ok := toMinor(whole, values.MinorUnitScale(currency))
	if !ok {
		return false
	}
	gap := minor - quotedMinor
	return gap > -values.MinorUnitScale(currency) && gap < values.MinorUnitScale(currency)
}

func setAmount(state State, change *deals.VerdictChange, amountMinor int64) string {
	if state.Seen.AmountMinor != nil && *state.Seen.AmountMinor == amountMinor && change.Currency == "" {
		return DroppedUnchanged
	}
	change.AmountMinor = amountMinor
	return ""
}

// convert takes a quoted amount into the deal's currency through the deals
// FX engine, then rounds half away from zero to whole units. The rates and
// their dates go in evidence.
func convert(amountMinor int64, from, to string, rates Rates, evidence map[string]any) (int64, bool, error) {
	fromRate, ok, err := rates.For(from)
	if err != nil || !ok {
		return 0, false, err
	}
	toRate, ok, err := rates.For(to)
	if err != nil || !ok {
		return 0, false, err
	}
	converted, err := deals.ConvertBetween(amountMinor, fromRate.Rate, toRate.Rate, from, to)
	if err != nil {
		return 0, false, err
	}
	scale := values.MinorUnitScale(to)
	whole := deals.DivRoundHalfAwayFromZero(big.NewInt(converted), big.NewInt(scale))
	minor, ok := toMinor(whole.Int64(), scale)
	if !ok {
		return 0, false, nil
	}
	evidence["rate_from"] = rateText(fromRate)
	evidence["rate_to"] = rateText(toRate)
	evidence["rate_date"] = rateDate(fromRate, toRate).Format(time.DateOnly)
	return minor, true, nil
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
