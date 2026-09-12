// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal's money is allowed to be, and how it refuses what it is not.
//
// Its own file because it is ONE rule with TWO writers: a deal is born through
// deal_create.go and re-priced through deal.go, and both ask exactly the
// function below. Before the recurring figure existed each spelled the rule
// out for itself and the two copies agreed by accident, because there was only
// one way to write "an amount and a currency come together". A second figure
// ended that accident, which is what TestOneFunctionDecidesWhetherADealsMoneyPairIsLegal
// now keeps shut.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// moneyPairError is the one statement of the deal money rule, shared by the
// create path and the update path so the two writers of one invariant cannot
// drift.
//
// Held by: TestOneFunctionDecidesWhetherADealsMoneyPairIsLegal
// (backend/gates/dealmoneypairwriters_test.go), which fails when a second Go
// site compares a money figure's nil-ness against the currency's. The SQL half
// — deal_money_currency_pair — is outside what that test can read, and is held
// against a live schema in the integration lane instead. It answers on the RESULTING row, never on the request: a patch that
// leaves a figure alone still has to produce a legal row.
//
// The rule the deal_money_currency_pair CHECK holds in SQL: the currency is
// present exactly when at least one figure is. A deal may carry a one-off
// amount alone, a recurring figure alone, or both, but never a figure without
// the currency to read it in, and never a stranded currency.
//
// Both figures are validated as values.Money against the same currency, which
// is what makes the minor-unit scale mean one thing on the row.
func moneyPairError(amount, arr *int64, currency *string) error {
	anyFigure := amount != nil || arr != nil
	if anyFigure != (currency != nil) {
		if !anyFigure {
			// A currency with nothing to price. The caller supplies a figure
			// or clears the currency; naming the amount points at the one a
			// caller is most likely to have meant.
			return &AmountCurrencyPairError{Missing: amountField}
		}
		return &AmountCurrencyPairError{Missing: currencyField}
	}
	if amount != nil {
		// One spelling of "a valid amount+currency" (values.Money), the same
		// rule the schema CHECKs repeat.
		if _, err := values.NewMoney(*amount, *currency); err != nil {
			return err
		}
	}
	if arr != nil {
		if *arr < 0 {
			return &NegativeArrError{}
		}
		if _, err := values.NewMoney(*arr, *currency); err != nil {
			return err
		}
	}
	return nil
}

// currencyRestatementError refuses a currency change that left a figure
// standing in the old currency.
//
// The numerals on a row do not carry their unit: changing the code alone
// re-denominates every figure already on the row, silently. 12,000 EUR becomes
// 12,000 JPY, which is about a twelfth of the price, and nothing in the row, the
// audit diff or the forecast history says the deal was repriced — only that its
// currency was corrected, which is what a caller fixing a typo believes they did.
//
// So a currency move requires every populated figure to be restated in the same
// request, even to the same numeral. Restating it makes the reprice explicit and
// puts it in the audit diff where a reconstruction can see it. Clearing the
// currency entirely is allowed only once no figure is left to denominate, which
// moneyPairError already enforces.
//
// `restated` says which of the three the request actually carried, so
// re-sending the currency a deal already holds is not a change and asks for
// nothing.
func currencyRestatementError(current crmcontracts.Deal,
	resultingCurrency *string, restated moneyRestatement,
) error {
	if !restated.Currency {
		return nil
	}
	// A currency CLEARED reinterprets nothing: moneyPairError has already
	// established that no figure is left for it to denominate, so there is
	// nothing to restate. This is the path a clear of the last figure takes,
	// and refusing it here would make that clear impossible — which would in
	// turn make the offer-accept refusal advice with no move behind it.
	if resultingCurrency == nil {
		return nil
	}
	if current.AmountMinor != nil && !restated.Amount {
		return &CurrencyRestatementError{Field: amountField}
	}
	if current.ExpectedArrMinor != nil && !restated.Arr {
		return &CurrencyRestatementError{Field: arrField}
	}
	return nil
}

// moneyRestatement says which of a deal's three money fields one request
// carried. Named rather than three bare bools at the call site, where
// `(true, false, true)` says nothing about which field is which and a
// transposed pair reads exactly like a correct one.
type moneyRestatement struct {
	Currency bool
	Amount   bool
	Arr      bool
}

// moneyMoved says which of a deal's money fields one patch actually CHANGED —
// the question moneyRestatement's neighbour asks about what a request carried.
// Named for that type's own reason: `(true, false)` at a call site says nothing
// about which field is which, and a transposed pair reads exactly like a
// correct one.
type moneyMoved struct {
	Arr      bool
	Currency bool
}

// CurrencyRestatementError names the figure the caller has to send again.
type CurrencyRestatementError struct{ Field string }

func (e *CurrencyRestatementError) Error() string {
	return "changing the currency requires restating " + e.Field +
		" in the new currency, because the stored number carries no unit of its own"
}

// FieldFault points at the figure rather than at the currency: the caller meant
// to change the currency, and the thing they still owe is the figure.
func (e *CurrencyRestatementError) FieldFault() (field, code, message string) {
	return e.Field, "currency_restatement_required", e.Error()
}

// patchedMoney reads one money column's RESULTING value out of a patch: the
// value the patch writes where it writes one, and the row's current value where
// it does not. A patch entry holding nil is a clear, which is exactly the state
// that cannot be read off the request struct.
func patchedMoney(after map[string]any, column string, current *int64) *int64 {
	v, ok := after[column]
	if !ok {
		return current
	}
	if minor, isInt := v.(int64); isInt {
		return &minor
	}
	return nil
}

// refuseManualArrEdit keeps an offer-derived recurring figure the offer's to
// state.
//
// A deal carrying arr_source_offer_id got its ARR from an accepted offer, and
// that offer is a signed document: its lines say what repeats and how often,
// and the annual figure follows from them. A human retyping the number would
// leave the deal claiming a recurring value its own source document does not
// support, with nothing on the record saying which of the two is right.
//
// Only a MOVE is refused. Re-sending the figure the deal already holds asks for
// nothing, so an unrelated edit that echoes the whole record back still goes
// through — which matters, because that is what an ordinary form save does.
//
// Every other field stays editable. This is a lock on one figure, not on the
// deal.
func refuseManualArrEdit(current crmcontracts.Deal, resultingArr *int64, moved moneyMoved) error {
	if current.ArrSourceOfferId == nil {
		return nil
	}
	// A CURRENCY move is an ARR move, even where the numeral does not change.
	// The stored figure carries no unit, so re-denominating it states a
	// different recurring value than the offer it names — 12,000 EUR read as
	// 12,000 USD is a different claim, and the source document still says
	// euros. Without this the restatement rule lets exactly that through: it
	// asks only that every figure be RESENT, and resending the same numeral
	// under a new code satisfies it.
	if !moved.Arr && !moved.Currency {
		return nil
	}
	return &ArrFromOfferError{
		Offer:   current.ArrSourceOfferId.String(),
		Cleared: moved.Arr && resultingArr == nil,
	}
}

// ArrFromOfferError refuses a manual edit to an offer-derived recurring figure.
type ArrFromOfferError struct {
	Offer string
	// Cleared distinguishes "you tried to change it" from "you tried to remove
	// it", because the two callers are doing different things and the second
	// is usually somebody trying to undo the accept.
	Cleared bool
}

func (e *ArrFromOfferError) Error() string {
	if e.Cleared {
		return "this deal's recurring revenue comes from an accepted offer and cannot be cleared by hand; " +
			"accepting a different offer replaces it"
	}
	return "this deal's recurring revenue comes from an accepted offer and cannot be edited by hand; " +
		"accepting a different offer replaces it"
}

// FieldFault points at the figure the caller tried to move.
func (e *ArrFromOfferError) FieldFault() (field, code, message string) {
	return arrField, "arr_from_offer", e.Error()
}

// ArrCurrencyConflictError refuses a currency change over a recurring figure
// that nobody restated. From and To name the two currencies so the refusal can
// say which move it stopped rather than only that it stopped one.
type ArrCurrencyConflictError struct{ From, To string }

func (e *ArrCurrencyConflictError) Error() string {
	return "the deal carries expected_arr_minor in " + e.From +
		" and this would move it to " + e.To + " without restating it"
}

// FieldFault points at the recurring figure, because clearing or restating it
// is the action that unblocks the caller. Pointing at the currency would ask
// them to undo the change they meant to make.
func (e *ArrCurrencyConflictError) FieldFault() (field, code, message string) {
	return arrField, "arr_currency_conflict", e.Error()
}

// NegativeArrError refuses recurring revenue below zero. A negative one-off
// amount is a credit somebody can argue for; a negative subscription is not a
// subscription, and deal_expected_arr_nonnegative refuses it in SQL either way.
type NegativeArrError struct{}

func (e *NegativeArrError) Error() string {
	return "expected_arr_minor cannot be negative"
}

// FieldFault points a 422 at the recurring figure itself.
func (e *NegativeArrError) FieldFault() (field, code, message string) {
	return arrField, "arr_negative", e.Error()
}

// AmountCurrencyPairError refuses a half-specified money value. Missing names
// the half that was NOT supplied, because that is the input the caller adds —
// telling someone who sent a currency to fix the currency is no guidance.
type AmountCurrencyPairError struct{ Missing string }

func (e *AmountCurrencyPairError) Error() string {
	return "currency comes with amount_minor or expected_arr_minor, and neither figure comes without it"
}

// FieldFault refuses an amount without its currency (or the reverse) — the pair is atomic.
func (e *AmountCurrencyPairError) FieldFault() (field, code, message string) {
	field = e.Missing
	if field == "" {
		field = currencyField
	}
	return field, "amount_currency_pair", e.Error()
}
