// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Worklist's pricing seam, against real stored rates.
//
// The engine's own reading — the as-of cutoff, newest-wins, the identity
// shortcut — is SQL, so a unit test over a stub cannot fail the binding. What
// this proves is the seam's whole contract in one call: a foreign amount
// converts at the stored rate, a base-currency amount converts at identity, and
// a currency the estate holds no rate for comes back unpriced rather than raw.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/attention"
)

func TestTheWorklistPricesAmountsThroughTheStoredRates(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('JPY', 'EUR', '0.0060000000', CURRENT_DATE - 1)`)

	converted, base, err := compose.AttentionBaseMoney{Pool: e.Pool}.ToBase(ctx, time.Now().UTC(),
		[]attention.CurrencyAmount{
			{Minor: 5_000_000, Currency: "JPY"},
			{Minor: 40_000, Currency: "EUR"},
			{Minor: 9_000_000, Currency: "GBP"},
		})
	if err != nil {
		t.Fatalf("pricing three amounts: %v", err)
	}

	if base != "EUR" {
		t.Fatalf("the seam prices into %q, wanted the installation's EUR", base)
	}
	if converted[0] == nil || *converted[0] != 30_000 {
		t.Fatalf("¥5,000,000 came back as %v, wanted 30000 at the stored 0.006", converted[0])
	}
	if converted[1] == nil || *converted[1] != 40_000 {
		t.Fatalf("a base-currency amount came back as %v, wanted itself", converted[1])
	}
	if converted[2] != nil {
		t.Fatalf("a currency with no stored rate came back priced at %d — that raw figure is exactly what the ordering must not see", *converted[2])
	}
}
