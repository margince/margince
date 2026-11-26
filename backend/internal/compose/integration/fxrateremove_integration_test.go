// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/currencylock"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *Env) setRate(t *testing.T, from, rate string) {
	t.Helper()
	if _, err := e.Deals.SetFxRate(e.Admin(), deals.SetFxRateInput{FromCurrency: from, Rate: rate}); err != nil {
		t.Fatalf("setting the %s rate to %s: %v", from, rate, err)
	}
}

// A removed currency leaves the sheet head, but its versions stay so a figure
// pinned to an earlier revision still finds the rate it used.
func TestARemovedCurrencyLeavesTheSheetAndKeepsItsHistory(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	e.setRate(t, "JPY", "0.0060")
	e.setRate(t, "USD", "0.9")

	if err := e.Deals.RemoveFxRate(ctx, "jpy"); err != nil {
		t.Fatalf("removing JPY: %v", err)
	}
	head, err := e.Deals.ListLatestFxRates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, listed := fxRateOf(head, "JPY"); listed {
		t.Error("the sheet head still lists the removed JPY")
	}
	if _, listed := fxRateOf(head, "USD"); !listed {
		t.Error("removing JPY took USD off the sheet")
	}
	if _, _, found, err := e.effectiveRate(t, "JPY"); err != nil || found {
		t.Errorf("a removed currency still converts: found=%v err=%v", found, err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'fx_rate' AND action = 'delete'
		AND before->>'from' = 'JPY' AND before->>'rate' = '0.0060000000' AND after IS NULL`); n != 1 {
		t.Errorf("delete audit rows with the removed rate and no after image = %d, want 1", n)
	}
	if err := e.Deals.RemoveFxRate(ctx, "JPY"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("removing a currency twice = %v, want ErrNotFound", err)
	}
}

func (e *Env) effectiveRate(t *testing.T, from string) (string, time.Time, bool, error) {
	t.Helper()
	var (
		rate  string
		asOf  time.Time
		found bool
	)
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) (err error) {
		rate, asOf, found, err = e.Deals.EffectiveFxRateInTx(e.Admin(), tx, from)
		return err
	})
	return rate, asOf, found, err
}

// Removal needs the fx_rate update grant, as correcting a rate does.
func TestRemovingACurrencyNeedsTheUpdateGrant(t *testing.T) {
	e := Setup(t)
	e.setRate(t, "USD", "0.9")
	for name, grant := range map[string]principal.ObjectGrant{
		"read only":   {Read: true},
		"create only": {Read: true, Create: true},
	} {
		ctx := e.As(e.Rep1, nil, fxRatePerms(grant))
		if err := e.Deals.RemoveFxRate(ctx, "USD"); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s: removal = %v, want ErrPermissionDenied", name, err)
		}
	}
	rows, err := e.Deals.ListLatestFxRates(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if _, listed := fxRateOf(rows, "USD"); !listed {
		t.Error("a refused removal took USD off the sheet")
	}
}

// The base currency lock counts the currencies on the sheet, not its versions,
// and a removal lifts it once the last one is gone.
func TestRemovingEveryCurrencyUnlocksTheBaseCurrency(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	store := identity.NewInstallationSettings(e.DB(), compose.NewSettingsStore(e.Pool), compose.StoredConfig(e.Pool))
	e.setRate(t, "USD", "0.9")
	e.setRate(t, "USD", "0.91")
	e.setRate(t, "JPY", "0.006")

	lock := func() currencylock.Lock {
		got, err := store.GetInstallation(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return got.BaseCurrencyLock
	}
	if got := lock(); got.Reason != currencylock.RateSheet || got.Count != 2 {
		t.Fatalf("lock = %+v, want the rate sheet counting 2 currencies though USD holds two versions", got)
	}
	if err := e.Deals.RemoveFxRate(ctx, "USD"); err != nil {
		t.Fatal(err)
	}
	if got := lock(); got.Count != 1 {
		t.Errorf("after removing USD the lock counts %d, want 1", got.Count)
	}
	if err := e.Deals.RemoveFxRate(ctx, "JPY"); err != nil {
		t.Fatal(err)
	}
	if got := lock(); got.Reason != "" {
		t.Errorf("an empty sheet still locks the base: %+v", got)
	}
	chf := "CHF"
	if _, err := store.UpdateInstallation(ctx, identity.InstallationPatch{BaseCurrency: &chf}); err != nil {
		t.Errorf("changing the base after removing every rate: %v", err)
	}
}
