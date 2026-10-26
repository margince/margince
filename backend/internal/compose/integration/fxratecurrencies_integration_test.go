// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The rate-sheet currency codes a currency picker reads: who may read them,
// and which rows of the sheet they come from.

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedSheet writes USD on two days and GBP on one through the real writer.
// The sheet head then holds two currencies, and one of them has a history.
func seedSheet(t *testing.T, e *Env) {
	t.Helper()
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	today := fxTestNow.Truncate(24 * time.Hour)
	for _, in := range []deals.SetFxRateInput{
		{FromCurrency: "USD", Rate: "0.91", EffectiveDate: today},
		{FromCurrency: "USD", Rate: "0.93", EffectiveDate: today.AddDate(0, 0, 1)},
		{FromCurrency: "GBP", Rate: "1.17", EffectiveDate: today},
	} {
		if _, err := e.Deals.SetFxRate(e.Admin(), in); err != nil {
			t.Fatalf("set %s on %s: %v", in.FromCurrency, in.EffectiveDate, err)
		}
	}
}

func TestARepReadsTheSheetCurrenciesButNotTheRates(t *testing.T) {
	e := Setup(t)
	seedSheet(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	_, codes, err := e.Deals.RateSheetCurrencies(rep)
	if err != nil {
		t.Fatalf("rep read of the sheet currencies: %v", err)
	}
	// USD has two dates and comes back once; the codes are in code order.
	if want := []string{"GBP", "USD"}; !slices.Equal(codes, want) {
		t.Fatalf("codes = %v, want %v", codes, want)
	}
	if _, err := e.Deals.ListLatestFxRates(rep); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep read of the rates = %v, want ErrPermissionDenied", err)
	}
}

// A custom role may hold deal read without installation_settings read. The
// picker still needs the base and the codes, so the base is read without that
// gate.
func TestDealReadAloneIsEnoughForTheSheetCurrencies(t *testing.T) {
	e := Setup(t)
	seedSheet(t, e)
	dealReader := e.As(e.Rep1, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{objDeal: {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	base, codes, err := e.Deals.RateSheetCurrencies(dealReader)
	if err != nil {
		t.Fatalf("deal-read-only caller: %v", err)
	}
	if base != "EUR" {
		t.Fatalf("base = %q, want EUR", base)
	}
	if want := []string{"GBP", "USD"}; !slices.Equal(codes, want) {
		t.Fatalf("codes = %v, want %v", codes, want)
	}

	noDeal := e.As(e.Rep1, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			objInstallSettings: {Read: true},
			"fx_rate":          {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	if _, _, err := e.Deals.RateSheetCurrencies(noDeal); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("caller without deal read = %v, want ErrPermissionDenied", err)
	}
}

func TestAnEmptySheetAnswersAnEmptyList(t *testing.T) {
	e := Setup(t)
	_, codes, err := e.Deals.RateSheetCurrencies(e.Admin())
	if err != nil {
		t.Fatalf("empty sheet: %v", err)
	}
	// A nil slice encodes as null, and the contract says the list is an array.
	if codes == nil || len(codes) != 0 {
		t.Fatalf("codes = %#v, want a non-nil empty slice", codes)
	}
}

// A row priced into another base is not on this sheet. The writer refuses a
// base change while any rate is priced, so it cannot leave such a row. One
// written straight in stands for a sheet older than that refusal.
func TestARateInAnotherBaseIsNotOnTheSheet(t *testing.T) {
	e := Setup(t)
	seedSheet(t, e)
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('JPY', 'USD', 0.0068, $1::date)`, fxTestNow)

	_, codes, err := e.Deals.RateSheetCurrencies(e.Admin())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := []string{"GBP", "USD"}; !slices.Equal(codes, want) {
		t.Fatalf("codes = %v, want %v: JPY converts into USD, not the EUR base", codes, want)
	}
}
