// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Worklist's pricing seam, against real stored rates.
//
// The engine's own reading — the as-of cutoff, newest-wins, the identity
// shortcut — is SQL, so a unit test over a stub cannot fail the binding. What
// this proves is the seam's whole contract in one call: a foreign amount
// converts at the stored rate and across the two currencies' minor-unit
// scales, a base-currency amount converts at identity, and a currency the
// estate holds no rate for comes back unpriced rather than raw.
//
// The foreign currency is JPY deliberately. It carries no minor unit where EUR
// carries two, so a conversion that multiplied minor units by a major-unit
// rate answers a hundredth of the truth here and is right for any same-scale
// pair — which is every pair in the demo data.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	if converted[0] == nil || *converted[0] != 3_000_000 {
		t.Fatalf("¥5,000,000 came back as %v EUR minor units, wanted 3000000 (€30,000) at the stored 0.006; "+
			"30000 would be €300, the answer a bare minor-unit multiply gives", converted[0])
	}
	if converted[1] == nil || *converted[1] != 40_000 {
		t.Fatalf("a base-currency amount came back as %v, wanted itself", converted[1])
	}
	if converted[2] != nil {
		t.Fatalf("a currency with no stored rate came back priced at %d — that raw figure is exactly what the ordering must not see", *converted[2])
	}
}

// The Worklist and the brief rank only ORDER by the base currency, so neither
// waits for a re-base in flight nor needs the installation_settings grant.
// The re-base holds the row as a settings write does. A read that took it FOR
// SHARE would park behind it until the bound below gives up.
func TestRankingByMoneyNeitherWaitsForAReBaseNorNeedsTheSettingsGrant(t *testing.T) {
	e := Setup(t)
	// The admin holds the settings grant, so its case reaches the row and
	// proves the lock half; the rep proves the grant half.
	seats := map[string]context.Context{
		"an admin": e.Admin(),
		"a seat holding only deal.read": e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true}},
			RowScope: principal.RowScopeAll,
		}),
	}

	rebase, err := OwnerConn(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("opening the re-base transaction: %v", err)
	}
	//craft:ignore swallowed-errors the re-base only holds the row; rolling it back is the cleanup
	defer func() { _ = rebase.Rollback(context.Background()) }()
	if err := settings.LockForWrite(e.Admin(), rebase, baseCurrencyKey); err != nil {
		t.Fatalf("holding the base-currency row as a re-base does: %v", err)
	}

	now := clocktest.Now(t)
	reads := map[string]func(context.Context) error{
		"the Worklist's pricing": func(ctx context.Context) error {
			_, _, err := compose.AttentionBaseMoney{Pool: e.Pool}.ToBase(ctx, now,
				[]attention.CurrencyAmount{{Minor: 40_000, Currency: "EUR"}})
			return err
		},
		"the brief rank": func(ctx context.Context) error {
			_, err := briefs.NewBriefEngine(e.Pool, contacts.NewStore(e.DB())).Rank(ctx, now)
			return err
		},
	}
	for readName, read := range reads {
		for seatName, seat := range seats {
			done := make(chan error, 1)
			go func() { done <- read(seat) }()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("%s, by %s: %v", readName, seatName, err)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%s, by %s, waited on the re-base's row lock: a read that only ranks took FOR SHARE",
					readName, seatName)
			}
		}
	}
}
