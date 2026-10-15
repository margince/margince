// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/url"
	"testing"
	"time"
)

// A handle pins the currency it converted to, so reopening it after the
// installation's base changed still reconciles to the figure that was printed.
// The as-of beside it is pinned for the same reason, and the two travel
// together.
func TestAHandlePinsTheCurrencyItConvertedTo(t *testing.T) {
	t.Parallel()
	minted := derivationURL("pipeline-current", nil, []string{"stage_id"},
		[]reportAggregate{{Fn: "sum", Field: "amount_base_minor", As: "value"}},
		map[string]any{"stage_id": "s1", "value": int64(5000)},
		time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), "EUR")

	values, err := url.ParseQuery(minted)
	if err != nil {
		t.Fatalf("the minted handle is not a query string: %v", err)
	}
	if got := values.Get(baseCurrencyKey); got != "EUR" {
		t.Errorf("%s = %q, want EUR: the detail would convert at whatever the installation holds now",
			baseCurrencyKey, got)
	}

	parsed, err := parseDerivationQuery(values)
	if err != nil {
		t.Fatalf("a handle this package minted does not resolve: %v", err)
	}
	if parsed.BaseCurrency != "EUR" {
		t.Errorf("parsed base currency = %q, want EUR", parsed.BaseCurrency)
	}
	if parsed.AsOf.IsZero() {
		t.Error("the pinned instant did not survive the round trip, so the two keys do not travel together")
	}
}

// A handle minted before this key resolves at the installation's current base,
// which is what it always did. The alternative is refusing every handle shared
// before the key existed.
func TestAHandleWithoutTheKeyLeavesTheFrameAlone(t *testing.T) {
	t.Parallel()
	values, err := url.ParseQuery(derivationURL("pipeline-current", nil, []string{"stage_id"},
		nil, map[string]any{"stage_id": "s1"}, time.Time{}, ""))
	if err != nil {
		t.Fatal(err)
	}
	if values.Has(baseCurrencyKey) {
		t.Errorf("an unpinned handle carries %s=%q, want the key absent",
			baseCurrencyKey, values.Get(baseCurrencyKey))
	}
	parsed, err := parseDerivationQuery(values)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.BaseCurrency != "" {
		t.Errorf("parsed base currency = %q, want empty so the frame is left alone", parsed.BaseCurrency)
	}
}

// The value reaches the frame the drill-through converts under, so its shape is
// checked rather than trusted.
func TestAMalformedPinnedCurrencyIsRefused(t *testing.T) {
	t.Parallel()
	for _, spelling := range []string{"eur", "EURO", "E", "", "EUR,USD"} {
		values := url.Values{baseCurrencyKey: {spelling}, "by": {"stage_id"}}
		if _, err := parseDerivationQuery(values); err == nil {
			t.Errorf("%q was accepted as a pinned currency", spelling)
		}
	}
	// Two values are one handle naming two currencies, which names neither.
	values := url.Values{baseCurrencyKey: {"EUR", "USD"}, "by": {"stage_id"}}
	if _, err := parseDerivationQuery(values); err == nil {
		t.Error("a handle pinning two currencies was accepted")
	}
}
