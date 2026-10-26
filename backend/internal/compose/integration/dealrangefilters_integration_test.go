// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rangeDeal creates an open deal on the valuation fixture's pipeline.
func (f valuationFixture) rangeDeal(t *testing.T, name string, in deals.CreateDealInput) ids.DealID {
	t.Helper()
	in.Name, in.PipelineID, in.StageID, in.Source = name, f.pipeline, f.open, "manual"
	if in.OwnerID == nil {
		in.OwnerID = new(ids.From[ids.UserKind](f.e.Rep1))
	}
	row, err := f.e.Deals.CreateDeal(f.e.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.DealKind](ids.UUID(row.Id))
}

// listedNames lists the deals a filter leaves, by name, in name order.
func listedNames(ctx context.Context, t *testing.T, e *Env, in deals.ListDealsInput) []string {
	t.Helper()
	rows, _, err := e.Deals.ListDeals(ctx, in)
	if err != nil {
		t.Fatalf("listing deals: %v", err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	slices.Sort(names)
	return names
}

func calendarDay(t *testing.T, iso string) *time.Time {
	t.Helper()
	day, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		t.Fatal(err)
	}
	return &day
}

func assertListed(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s listed %v, want %v", what, got, want)
	}
}

// The close-date range reads the Close column's day. A won deal counts by the
// day it closed, an open deal by its expected date, an undated deal by neither.
func TestTheDealListNarrowsByCloseDay(t *testing.T) {
	f := setupValuation(t)
	f.rangeDeal(t, "February", deals.CreateDealInput{ExpectedClose: calendarDay(t, "2020-02-10")})
	f.rangeDeal(t, "May", deals.CreateDealInput{ExpectedClose: calendarDay(t, "2020-05-01")})
	f.rangeDeal(t, "Undated", deals.CreateDealInput{})
	// Expected in June, closed on the fixture's day: the range must read the close.
	won := f.rangeDeal(t, "Won early", deals.CreateDealInput{ExpectedClose: calendarDay(t, "2020-06-15")})
	f.close(t, won)

	quarter := deals.RangeFilters{CloseFrom: calendarDay(t, "2020-01-01"), CloseTo: calendarDay(t, "2020-03-31")}
	assertListed(t, "the first quarter", listedNames(f.e.Admin(), t, f.e, deals.ListDealsInput{RangeFilters: quarter}),
		"February", "Won early")
	from := deals.RangeFilters{CloseFrom: calendarDay(t, "2020-04-01")}
	assertListed(t, "close_from alone", listedNames(f.e.Admin(), t, f.e, deals.ListDealsInput{RangeFilters: from}),
		"May")
	to := deals.RangeFilters{CloseTo: calendarDay(t, "2020-02-10")}
	assertListed(t, "an inclusive close_to", listedNames(f.e.Admin(), t, f.e, deals.ListDealsInput{RangeFilters: to}),
		"February", "Won early")
}

// The value range compares the base-currency value the Value sort orders by.
// Mixed currencies meet on one scale, and an unpriceable deal is in no range.
func TestTheDealListNarrowsByValueAcrossCurrencies(t *testing.T) {
	f := setupValuation(t)
	f.rate(t, "USD", "0.5", valuationDate)
	f.rate(t, "JPY", "0.006", valuationDate)
	f.rangeDeal(t, "EUR 300", deals.CreateDealInput{AmountMinor: new(int64(30000)), Currency: new("EUR")})
	f.rangeDeal(t, "USD 500", deals.CreateDealInput{AmountMinor: new(int64(50000)), Currency: new("USD")})
	f.rangeDeal(t, "JPY 50000", deals.CreateDealInput{AmountMinor: new(int64(50000)), Currency: new("JPY")})
	f.rangeDeal(t, "GBP without a rate", deals.CreateDealInput{AmountMinor: new(int64(70000)), Currency: new("GBP")})
	f.rangeDeal(t, "Unpriced", deals.CreateDealInput{})

	list := func(r deals.RangeFilters) []string {
		return listedNames(f.e.Admin(), t, f.e, deals.ListDealsInput{RangeFilters: r})
	}
	// USD 500 is 250.00 in the base currency and JPY 50000 is 300.00.
	assertListed(t, "value_min 260.00", list(deals.RangeFilters{ValueMin: new(int64(26000))}),
		"EUR 300", "JPY 50000")
	assertListed(t, "value_max 299.99", list(deals.RangeFilters{ValueMax: new(int64(29999))}),
		"USD 500")
	assertListed(t, "250.00 to 300.00", list(deals.RangeFilters{ValueMin: new(int64(25000)), ValueMax: new(int64(30000))}),
		"EUR 300", "JPY 50000", "USD 500")
	assertListed(t, "currency USD", list(deals.RangeFilters{Currency: new("USD")}), "USD 500")
}

// rangeReaderPerms reads every deal, and every company capture privacy lets it
// see. So a deal it may open can name a company it may not.
var rangeReaderPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"deal":    {Read: true},
		"company": {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

// The country is the company's address country. A deal with no company has no
// country, and a company the reader may not see does not answer for its deal.
func TestTheDealListNarrowsByTheCompanyCountry(t *testing.T) {
	f := setupValuation(t)
	country := func(name, code string, owner ids.UUID) *ids.CompanyID {
		company, err := f.e.Contacts.CreateCompany(f.e.Admin(), contacts.CreateCompanyInput{
			DisplayName: name, OwnerID: userIDPtr(&owner), Address: &crmcontracts.Address{Country: new(code)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return new(ids.From[ids.CompanyKind](ids.UUID(company.Id)))
	}
	f.rangeDeal(t, "Berlin", deals.CreateDealInput{CompanyID: country("Berlin GmbH", "DE", f.e.Rep1)})
	munich := country("Munich GmbH", "DE", f.e.Rep3)
	f.rangeDeal(t, "Munich", deals.CreateDealInput{CompanyID: munich})
	// A captured company still private to the rep who captured it.
	f.e.WsExec(t, `UPDATE company SET visibility = 'owner' WHERE id = $1`, munich.UUID)
	f.rangeDeal(t, "Hanoi", deals.CreateDealInput{CompanyID: country("Hanoi JSC", "VN", f.e.Rep1)})
	f.rangeDeal(t, "No company", deals.CreateDealInput{})

	germany := deals.ListDealsInput{Country: new("DE")}
	captor := f.e.As(f.e.Rep3, nil, rangeReaderPerms)
	assertListed(t, "country DE for the rep who captured Munich", listedNames(captor, t, f.e, germany), "Berlin", "Munich")
	rep := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, rangeReaderPerms)
	assertListed(t, "country DE for a rep who cannot see Munich", listedNames(rep, t, f.e, germany), "Berlin")

	dealsOnly := principal.Permissions{
		Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}, RowScope: principal.RowScopeAll,
	}
	_, _, err := f.e.Deals.ListDeals(f.e.As(f.e.Rep1, nil, dealsOnly), germany)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat that may not read companies filtered by their country: %v, want permission denied", err)
	}
}
