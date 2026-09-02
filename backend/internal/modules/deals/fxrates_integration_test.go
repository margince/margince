// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// The lookup half of the conversion engine, against a real database.
//
// The arithmetic is proven without one (fxconvert_test.go) because it is
// arithmetic. This is the half that cannot be: which row the as-of cutoff and
// newest-wins select, and — the reason this file exists at all — that a bare
// DATE column decodes into the time.Time the engine scans it into. Two files in
// this tree disagreed about that in prose, and only a read against real rows
// settles it.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// seedRate writes one effective-dated rate straight in, so the test arranges
// what it means rather than through the write path's own guards.
func (e *configEnv) seedRate(t *testing.T, from string, rate string, on time.Time) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ($1, 'EUR', $2::numeric, $3::date)
		ON CONFLICT (from_currency, to_currency, rate_date) DO UPDATE SET rate = excluded.rate`,
		from, rate, on.Format(time.DateOnly)); err != nil {
		t.Fatalf("seeding the %s rate for %s: %v", from, on.Format(time.DateOnly), err)
	}
}

// TestTheEngineReadsTheNewestRateOnOrBeforeTheAsOfDay holds the two decisions
// the lookup makes, and the decode that carries the answer back.
func TestTheEngineReadsTheNewestRateOnOrBeforeTheAsOfDay(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()

	asOf := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	e.seedRate(t, "JPY", "0.0055", asOf.AddDate(0, 0, -5))
	e.seedRate(t, "JPY", "0.0061", asOf.AddDate(0, 0, -1)) // the newest on or before
	e.seedRate(t, "JPY", "0.9999", asOf.AddDate(0, 0, 3))  // after: must not be chosen

	var got FXRate
	var found bool
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		got, found, err = NewFXRates("EUR", asOf).For(ctx, tx, "JPY")
		return err
	}); err != nil {
		t.Fatalf("reading the JPY rate: %v", err)
	}
	if !found {
		t.Fatal("no rate found for a currency three rates were seeded for")
	}
	// The DATE column, decoded. This assertion is the file's reason: a bare date
	// that did not decode into time.Time would have failed the scan above, and
	// no unit test over the arithmetic could have found it.
	if want := asOf.AddDate(0, 0, -1); !got.On.Equal(want) {
		t.Errorf("the rate is dated %s, want %s — the newest ON OR BEFORE the as-of day, never one after it",
			got.On.Format(time.DateOnly), want.Format(time.DateOnly))
	}
	// ¥100 at 0.0061 is €0.61, which is 61 EUR minor units. JPY carries no
	// minor unit and EUR carries two, so both scales are in this number: a
	// conversion that multiplied the minor units alone would answer 1.
	converted, err := ConvertToBase(100, got.Rate, "JPY", "EUR")
	if err != nil {
		t.Fatalf("converting at the read rate: %v", err)
	}
	if converted != 61 {
		t.Errorf("¥100 at the read rate = %d EUR minor units, want 61 (€0.61) — either the rate that came "+
			"back is not the one seeded for that day, or the minor-unit scales were not both applied", converted)
	}
}

// A currency the estate holds no rate for answers "not found" rather than an
// error or an invented rate. The caller's policy decides what that means, and
// both callers have a different one.
func TestTheEngineAnswersNotFoundForAnUnpricedCurrency(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()

	var found bool
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		_, found, err = NewFXRates("EUR", time.Now().UTC()).For(ctx, tx, "XPF")
		return err
	}); err != nil {
		t.Fatalf("reading an unpriced currency: %v", err)
	}
	if found {
		t.Error("a currency with no stored rate answered found — nothing may invent a rate")
	}
}

// The base currency converts at the identity, without a row and without a
// query. Every caller would otherwise compare currencies itself, which is how
// one of them comes to compare them differently.
func TestTheBaseCurrencyNeedsNoStoredRate(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.as()

	var got FXRate
	var found bool
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		got, found, err = NewFXRates("EUR", time.Now().UTC()).For(ctx, tx, "EUR")
		return err
	}); err != nil {
		t.Fatalf("reading the base currency: %v", err)
	}
	if !found {
		t.Fatal("the base currency answered not found")
	}
	converted, err := ConvertToBase(123456, got.Rate, "EUR", "EUR")
	if err != nil {
		t.Fatalf("converting at the identity rate: %v", err)
	}
	if converted != 123456 {
		t.Errorf("the base currency converted %d to %d, want a passthrough", 123456, converted)
	}
}
