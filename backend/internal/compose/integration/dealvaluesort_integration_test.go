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

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// valueSortDeal is one deal the Value sort tests seed.
type valueSortDeal struct {
	name        string
	amountMinor int64
	currency    string
}

// In these deals the raw minor-unit order and the base-currency order disagree
// at every step. Yen and dong have no minor unit and the dinar has three. The
// Swiss deal has no rate at all.
var valueSortDeals = []valueSortDeal{
	{"Euro thousand", 100_000, "EUR"},          // €1,000.00
	{"Yen", 150_000, "JPY"},                    // ¥150,000 → €900.00
	{"Dinar", 1_234_000, "KWD"},                // 1,234.000 KWD → €3,393.50
	{"Dong", 50_000_000, "VND"},                // ₫50,000,000 → €1,750.00
	{"Franc without a rate", 9_999_999, "CHF"}, // no CHF rate: unpriceable
}

// valueSortRates prices every valueSortDeals currency except the franc.
func valueSortRates() map[string]string {
	return map[string]string{"JPY": "0.006", "KWD": "2.75", "VND": "0.000035"}
}

func TestSortingDealsByValueComparesThemInTheBaseCurrency(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	pipeline := seedValueSortDeals(t, e, fxTestNow.Truncate(24*time.Hour), valueSortRates(), valueSortDeals)

	for _, tc := range []struct {
		sort string
		want []string
	}{
		{"-amount_minor", []string{"Dinar", "Dong", "Euro thousand", "Yen", "Franc without a rate"}},
		{"amount_minor", []string{"Yen", "Euro thousand", "Dong", "Dinar", "Franc without a rate"}},
	} {
		if got := valueSortedNames(e.Admin(), t, e, pipeline, tc.sort); !slices.Equal(got, tc.want) {
			t.Errorf("sort=%s read %v, want %v", tc.sort, got, tc.want)
		}
	}
}

// The sort reads the base currency and the zone without the
// installation_settings gate, so a seat holding only deal.read can use it.
func TestASeatWithoutInstallationSettingsCanSortDealsByValue(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	pipeline := seedValueSortDeals(t, e, fxTestNow.Truncate(24*time.Hour), valueSortRates(), valueSortDeals)

	perms := activityLifecyclePerms
	perms.Objects = map[string]principal.ObjectGrant{"deal": {Read: true}, "pipeline": {Read: true}}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	want := []string{"Dinar", "Dong", "Euro thousand", "Yen", "Franc without a rate"}
	if got := valueSortedNames(rep, t, e, pipeline, "-amount_minor"); !slices.Equal(got, want) {
		t.Errorf("the rep's Value sort read %v, want %v", got, want)
	}
}

// The base value is converted from the amount and the currency, so a role that
// masks only the currency is refused the Value sort too. Ordered beside the
// visible amounts, the base values would give the masked currency away.
func TestACurrencyMaskRefusesTheValueSort(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	pipeline := seedValueSortDeals(t, e, fxTestNow.Truncate(24*time.Hour), valueSortRates(), valueSortDeals)

	perms := activityLifecyclePerms
	perms.Objects = map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}, "pipeline": {Read: true}}
	perms.FieldMasks = []principal.FieldMask{
		{Object: "deal", Field: "currency", Condition: principal.MaskOutsideWriteAuthority},
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	for _, sort := range []string{"-amount_minor", "amount_minor"} {
		_, _, err := e.Deals.ListDeals(rep, deals.ListDealsInput{PipelineID: &pipeline, Sort: &sort})
		refused, ok := errors.AsType[*values.ParseError](err)
		if !ok || refused.Code != "field_masked" {
			t.Errorf("sort=%s under a currency mask → %v, want the field_masked refusal", sort, err)
		}
	}
}

// Rates take effect by the installation's calendar day. At 12:00 UTC it is
// already the next day on Kiritimati (UTC+14), so the rate dated that day
// applies. A UTC "today" finds no rate and puts the yen deal last.
func TestTheValueSortConvertsAtTodaysRateInTheInstallationZone(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Pacific/Kiritimati"'::jsonb WHERE key = 'installation.timezone'`)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	localToday := fxTestNow.Truncate(24*time.Hour).AddDate(0, 0, 1)
	pipeline := seedValueSortDeals(t, e, localToday, map[string]string{"JPY": "0.01"}, []valueSortDeal{
		{"Euro thousand", 100_000, "EUR"}, // €1,000.00
		{"Yen", 150_000, "JPY"},           // ¥150,000 → €1,500.00
	})

	want := []string{"Yen", "Euro thousand"}
	if got := valueSortedNames(e.Admin(), t, e, pipeline, "-amount_minor"); !slices.Equal(got, want) {
		t.Errorf("sort=-amount_minor read %v, want %v", got, want)
	}
}

// A page cursor names the day its values were converted on. The next day's
// rates may differ, so the old cursor is refused instead of skipping or
// repeating deals.
func TestAValueSortCursorFromAnotherDayIsRefused(t *testing.T) {
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return fxTestNow })
	pipeline := seedValueSortDeals(t, e, fxTestNow.Truncate(24*time.Hour), valueSortRates(), valueSortDeals)

	sort, limit := "-amount_minor", 2
	_, page, err := e.Deals.ListDeals(e.Admin(), deals.ListDealsInput{PipelineID: &pipeline, Sort: &sort, Limit: &limit})
	if err != nil || !page.HasMore {
		t.Fatalf("first page: has more %t, err %v", page.HasMore, err)
	}
	e.Deals.WithClock(func() time.Time { return fxTestNow.AddDate(0, 0, 1) })
	_, _, err = e.Deals.ListDeals(e.Admin(), deals.ListDealsInput{
		PipelineID: &pipeline, Sort: &sort, Limit: &limit, Cursor: &page.NextCursor,
	})
	if _, ok := errors.AsType[*storekit.CursorSortMismatchError](err); !ok {
		t.Errorf("yesterday's cursor → %v, want CursorSortMismatchError", err)
	}
}

// seedValueSortDeals sets each rate effective on rateDay and creates the deals
// in one fresh pipeline, all through the real writers.
func seedValueSortDeals(
	t *testing.T, e *Env, rateDay time.Time, rates map[string]string, seeds []valueSortDeal,
) ids.PipelineID {
	t.Helper()
	admin := e.Admin()
	for currency, rate := range rates {
		if _, err := e.Deals.SetFxRate(admin, deals.SetFxRateInput{FromCurrency: currency, Rate: rate, EffectiveDate: rateDay}); err != nil {
			t.Fatalf("setting the %s rate: %v", currency, err)
		}
	}
	pipeline, open, _ := DealFixture(t, e)
	for _, seed := range seeds {
		if _, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
			Name: seed.name, PipelineID: pipeline, StageID: open,
			AmountMinor: &seed.amountMinor, Currency: &seed.currency,
		}); err != nil {
			t.Fatalf("creating %s: %v", seed.name, err)
		}
	}
	return pipeline
}

// valueSortedNames walks the whole list two deals at a time. The order it
// returns is the one the keyset cursor continues, not only the first page's.
func valueSortedNames(ctx context.Context, t *testing.T, e *Env, pipeline ids.PipelineID, sort string) []string {
	t.Helper()
	limit := 2
	var names []string
	var cursor *string
	for range len(valueSortDeals) {
		page, next, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{
			PipelineID: &pipeline, Sort: &sort, Limit: &limit, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("listing by %s: %v", sort, err)
		}
		for _, d := range page {
			names = append(names, d.Name)
		}
		if !next.HasMore {
			return names
		}
		cursor = &next.NextCursor
	}
	t.Fatalf("listing by %s did not end after %d pages: %v", sort, len(valueSortDeals), names)
	return nil
}
