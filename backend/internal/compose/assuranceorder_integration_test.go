// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Findings of one severity rank by their amount in the base currency, and a
// finding in a currency with no rate comes after every priced one. The raw
// amounts here run in the opposite order, so a ranking on them fails.
func TestFindingsRankByTheirAmountInTheBaseCurrency(t *testing.T) {
	e := setupForecast(t)
	seedRate(t, e, "0.5", 1)

	finding := func(name string, amountMinor int64, currency string) ids.UUID {
		deal := e.seedOpenDeal(t, name, pipelineTestStage, &e.Rep1, nil, nil)
		if _, err := e.owner.Exec(t.Context(),
			`INSERT INTO assurance_exception
			   (logical_key, type, subject_kind, subject_id, severity, affected_minor, currency, captured_by)
			 VALUES ($1, 'close_past', 'deal', $2, 'high', $3, $4, 'test')`,
			"close_past:"+deal.String(), deal, amountMinor, currency); err != nil {
			t.Fatal(err)
		}
		return deal
	}
	unpriced := finding("Yen deal", 9_000_000, "JPY")
	usd := finding("Dollar deal", 80_000, "USD")
	eur := finding("Euro deal", 50_000, "EUR")

	admin := e.Admin()
	var got []ids.UUID
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		found, err := AssuranceExceptions(admin, tx)
		for _, f := range found {
			got = append(got, f.SubjectID)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// 500 EUR, then 800 USD at 0.5 (400 EUR), then the yen with no rate.
	if want := []ids.UUID{eur, usd, unpriced}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want euro %v, dollar %v, yen %v", got, eur, usd, unpriced)
	}
}
