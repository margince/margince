// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// How an invoice reaches the base currency: which rate it asks for, what it
// freezes, and what the summary hands the conversion. The rate sheet and the
// arithmetic are seams, so these cases record what crossed them.

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rateAsked records the one question a RateOnFunc was asked.
type rateAsked struct {
	currency, base string
	day            time.Time
}

func numeric(digits int64, exp int32) pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(digits), Exp: exp, Valid: true}
}

func TestAnInvoiceAsksForTheRateInForceOnItsIssueDay(t *testing.T) {
	issued := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)
	var asked []rateAsked
	store := &Store{rates: Rates{RateOn: func(
		_ context.Context, _ pgx.Tx, currency, base string, day time.Time,
	) (Rate, bool, error) {
		asked = append(asked, rateAsked{currency, base, day})
		return Rate{Value: numeric(6, -3), On: day.AddDate(0, 0, -3)}, true, nil
	}}}

	rate, found, err := store.rateFor(context.Background(), nil,
		SourceInvoice{Currency: "jpy", IssuedOn: issued}, "EUR")
	if err != nil || !found {
		t.Fatalf("rateFor = %v, %v; want the sheet's rate", found, err)
	}
	if len(asked) != 1 || asked[0] != (rateAsked{"JPY", "EUR", issued}) {
		t.Fatalf("asked %+v, want one question for JPY into EUR on the issue day", asked)
	}
	if !rate.On.Equal(issued.AddDate(0, 0, -3)) {
		t.Errorf("rate dated %v, want the day the sheet dated it", rate.On)
	}
}

func TestAnUnknownBaseCurrencyHasNoRate(t *testing.T) {
	store := &Store{rates: Rates{RateOn: func(
		context.Context, pgx.Tx, string, string, time.Time,
	) (Rate, bool, error) {
		t.Fatal("asked the rate sheet for a conversion into no currency")
		return Rate{}, false, nil
	}}}
	if _, found, err := store.rateFor(context.Background(), nil,
		SourceInvoice{Currency: "EUR", IssuedOn: asOf}, ""); found || err != nil {
		t.Fatalf("rateFor with no base = %v, %v; want no rate and no error", found, err)
	}
}

// The rate and the day it is dated are one fact, and the schema's CHECK wants
// both or neither. A test asserting only the rate would pass while every
// insert aborted.
func TestTheFrozenRateTravelsWithItsDate(t *testing.T) {
	invoice := SourceInvoice{ExternalID: "INV-1", Currency: "USD", IssuedOn: asOf}
	rate := Rate{Value: numeric(92, -2), On: asOf.AddDate(0, 0, -10)}

	priced := deriveValues(invoice, asOf, map[string]ids.UUID{}, 0, &rate)
	if !priced.fxRate.Valid || priced.fxRate.Int.Cmp(big.NewInt(92)) != 0 || priced.fxRate.Exp != -2 {
		t.Errorf("froze %+v, want the sheet's 0.92 exactly", priced.fxRate)
	}
	if priced.fxDate == nil || !priced.fxDate.Equal(rate.On) {
		t.Errorf("froze the date %v, want the rate's own date %v", priced.fxDate, rate.On)
	}

	unpriced := deriveValues(invoice, asOf, map[string]ids.UUID{}, 0, nil)
	if unpriced.fxRate.Valid || unpriced.fxDate != nil {
		t.Errorf("an invoice with no rate froze %+v @ %v, want neither", unpriced.fxRate, unpriced.fxDate)
	}
}

// conversionAsked records one amount a ToBaseFunc was handed.
type conversionAsked struct {
	amount   int64
	from, to string
}

func TestTheSummaryConvertsEveryAmountFromItsOwnCurrency(t *testing.T) {
	var asked []conversionAsked
	toBase := func(amount int64, _ pgtype.Numeric, from, base string) (int64, error) {
		asked = append(asked, conversionAsked{amount, from, base})
		return amount * 10, nil
	}
	stored := storedInvoice{
		currency: "JPY", netMinor: 1000, creditedMinor: 200, openMinor: 300,
		rate: numeric(6, -3),
	}

	got, err := stored.inBase("EUR", toBase)
	if err != nil {
		t.Fatal(err)
	}
	want := []conversionAsked{{1000, "JPY", "EUR"}, {200, "JPY", "EUR"}, {300, "JPY", "EUR"}}
	if len(asked) != len(want) {
		t.Fatalf("converted %+v, want %+v", asked, want)
	}
	for i := range want {
		if asked[i] != want[i] {
			t.Errorf("conversion %d = %+v, want %+v", i, asked[i], want[i])
		}
	}
	if got.NetMinorBase != 10000 || got.CreditedMinorBase != 2000 || got.OpenMinorBase != 3000 {
		t.Errorf("base amounts = %d/%d/%d, want what the conversion answered",
			got.NetMinorBase, got.CreditedMinorBase, got.OpenMinorBase)
	}
	if got.RateMissing {
		t.Error("a converted invoice is marked as missing its rate")
	}
}

// An invoice with no rate reaches the formulas marked, so they refuse the
// whole figure instead of summing it in its issued currency.
func TestAnInvoiceWithNoRateIsMarkedRatherThanConverted(t *testing.T) {
	toBase := func(int64, pgtype.Numeric, string, string) (int64, error) {
		t.Fatal("converted an invoice that has no rate")
		return 0, nil
	}
	got, err := storedInvoice{currency: "USD", netMinor: 500, openMinor: 500}.inBase("EUR", toBase)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RateMissing {
		t.Fatal("an invoice with no rate is not marked, so a total would include it unconverted")
	}
	if got.OpenMinorBase != 500 {
		t.Errorf("open = %d, want the issued 500 kept so the open-balance fold still sees it", got.OpenMinorBase)
	}
}

func TestAConversionThatFailsFailsTheRead(t *testing.T) {
	refused := errors.New("rate is not finite")
	toBase := func(int64, pgtype.Numeric, string, string) (int64, error) { return 0, refused }
	_, err := storedInvoice{currency: "USD", netMinor: 500, rate: numeric(1, 0)}.inBase("EUR", toBase)
	if !errors.Is(err, refused) {
		t.Fatalf("inBase = %v, want the conversion's own error", err)
	}
}
