// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The rule that an offer's currency is not a way to reprice it.
//
// Its own file because it is one concept and offer.go had grown past the length
// a reader can hold at once: the refusal, what the caller is told to do instead,
// and the fault it maps to travel together and change together.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// refuseRepricingByCurrency refuses a currency change that would re-mean prices
// the caller did not restate.
//
// A line's unit_price_minor is an integer carrying no unit of its own, so moving
// the offer's currency leaves every one of them where it is and silently reads
// it in the new one: a 9500 that meant EUR 95.00 becomes USD 95.00 between
// same-scale currencies, and ₫9,500 where the minor-unit counts differ. Nothing
// in the row, the audit diff or the rendering reports a reprice — a caller
// fixing a dropdown believes they corrected a label.
//
// REFUSED rather than converted. Converting means choosing an FX rate and an
// as-of day for a document nobody has sent, so a quote's prices would move
// because somebody changed a dropdown; the deal surface already refuses the
// same move for the same reason (CurrencyRestatementError). Offers cannot ask
// for the figures back in the same request the way a deal can — the prices are
// rows of their own — so the refusal names what to do instead.
//
// An unpriced draft changes currency freely: with no figure to denominate there
// is nothing to re-mean. Re-sending the currency the offer already holds is not
// a change and asks for nothing.
func refuseRepricingByCurrency(
	ctx context.Context, tx pgx.Tx, current crmcontracts.Offer, next string,
) error {
	if next == current.Currency {
		return nil
	}
	// Non-zero is the whole test, because `unit_price_minor` is NOT NULL: a line
	// has no way to say "no price" except by stating 0, and zero is the one
	// integer that means the same amount in every currency. So the count asks
	// how many lines state a figure a currency change could re-mean, and both
	// kinds of zero answer no — a line priced at nothing on purpose, and the
	// honest zero an ungrounded AI draft writes for a price it will not guess
	// (`offer_line_item_ungrounded_price_zero` requires that sentinel to be 0).
	var priced int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM offer_line_item WHERE offer_id = $1 AND unit_price_minor <> 0`,
		current.Id).Scan(&priced); err != nil {
		return fmt.Errorf("counting the offer's priced lines: %w", err)
	}
	if priced == 0 {
		return nil
	}
	return &OfferCurrencyRepricingError{From: current.Currency, To: next, PricedLines: priced}
}

// OfferCurrencyRepricingError names what the caller has to do instead, because a
// refusal that only says no lands the whole cost on whoever is repricing.
type OfferCurrencyRepricingError struct {
	From, To    string
	PricedLines int
}

func (e *OfferCurrencyRepricingError) Error() string {
	return fmt.Sprintf(
		"changing this draft's currency from %s to %s would re-mean %d priced line(s) without "+
			"restating them, because a line's price carries no unit of its own — remove the "+
			"lines and re-enter them in %s, or start a new offer in %s",
		e.From, e.To, e.PricedLines, e.To, e.To)
}

// FieldFault points at the currency, which is the field the caller sent: unlike
// a deal, there is no figure on this request they could restate instead.
func (e *OfferCurrencyRepricingError) FieldFault() (field, code, message string) {
	return currencyField, "offer_currency_repricing_refused", e.Error()
}
