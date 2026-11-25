// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
)

// A day holds one current rate however often it is corrected. The earlier
// versions stay, and the database refuses a second current row.
func TestADayHoldsExactlyOneCurrentRate(t *testing.T) {
	e := Setup(t)
	for _, rate := range []string{"0.5", "0.9", "0.7"} {
		if _, err := e.Deals.SetFxRate(e.Admin(), deals.SetFxRateInput{FromCurrency: "USD", Rate: rate}); err != nil {
			t.Fatalf("setting the USD rate to %s: %v", rate, err)
		}
	}
	var current, versions int
	if err := e.Pool.QueryRow(e.Admin(), `
		SELECT count(*) FILTER (WHERE superseded_at IS NULL), count(*)
		FROM fx_rate WHERE from_currency = 'USD'`).Scan(&current, &versions); err != nil {
		t.Fatal(err)
	}
	if current != 1 || versions != 3 {
		t.Errorf("USD holds %d current of %d versions, want 1 of 3", current, versions)
	}
	if _, err := e.Pool.Exec(e.Admin(), `
		INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		SELECT from_currency, to_currency, 0.1, rate_date FROM fx_rate
		WHERE from_currency = 'USD' AND superseded_at IS NULL`); err == nil {
		t.Error("a second current USD rate for the same day was accepted")
	}
}

// A report's snapshot pins the rate sheet revision it saw. A correction that
// commits mid-report is invisible to it. A later detail at that revision reads
// the rate the report used.
func TestASnapshotPinsTheRateRevisionItSawAcrossACommittedCorrection(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	set := func(rate string) {
		if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "JPY", Rate: rate}); err != nil {
			t.Fatalf("setting the JPY rate to %s: %v", rate, err)
		}
	}
	rateAt := func(tx pgx.Tx, revision time.Time) string {
		var rate string
		err := tx.QueryRow(ctx, `SELECT rate::text FROM fx_rate WHERE from_currency = 'JPY' AND `+deals.RateVersionSQL("", "$1"), revision).Scan(&rate)
		if err != nil {
			t.Fatalf("reading the JPY rate at %s: %v", revision, err)
		}
		return rate
	}
	set("0.0060")

	var pinned time.Time
	err := database.WithWorkspaceSnapshot(ctx, e.Pool, func(inner context.Context) error {
		return database.WithWorkspaceTx(inner, e.Pool, func(tx pgx.Tx) error {
			var err error
			if pinned, err = deals.RateSheetRevision(inner, tx); err != nil {
				return err
			}
			set("0.0070")
			if got := rateAt(tx, pinned); got != "0.0060000000" {
				t.Errorf("inside the snapshot the pinned revision read %s, want the rate the snapshot saw", got)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if got := rateAt(tx, pinned); got != "0.0060000000" {
			t.Errorf("a later detail at the pinned revision read %s, want 0.0060000000", got)
		}
		current, err := deals.RateSheetRevision(ctx, tx)
		if err != nil {
			return err
		}
		if got := rateAt(tx, current); got != "0.0070000000" {
			t.Errorf("a new report's revision read %s, want the corrected 0.0070000000", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
