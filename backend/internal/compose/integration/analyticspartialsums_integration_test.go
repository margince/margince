// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A partial converted sum states what it leaves out for the deals its reader may see.
// A reader masked from the amount or the currency gets no sum, counts or currencies.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func askBaseSum(ctx context.Context, t *testing.T, e *Env) compose.AnalyticsAnswer {
	t.Helper()
	var answer compose.AnalyticsAnswer
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = compose.RunAnalyticsQuery(ctx, tx, analyticsquery.Query{
			Entity:   "deals-by-stage",
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: "amount_base_minor", As: "base"}},
		}, analyticsquery.Floor(1))
		return err
	})
	if err != nil {
		t.Fatalf("asking the converted sum: %v", err)
	}
	return answer
}

func TestAReaderMaskedFromTheAmountOrCurrencyGetsNoSumCountsOrCurrencies(t *testing.T) {
	for _, field := range []string{"", "amount_minor", "currency"} {
		t.Run("mask on "+field, func(t *testing.T) {
			e := Setup(t)
			e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
				VALUES ('USD', 'EUR', '0.5', CURRENT_DATE - 1)`)
			pipeline, open, _ := DealFixture(t, e)
			for name, money := range map[string][2]any{"Rated": {10_000, "USD"}, "Unrated": {4_000, "CHF"}} {
				id := e.SeedDeal(t, name, pipeline, open, &e.Rep1)
				e.WsExec(t, `UPDATE deal SET amount_minor = $2, currency = $3 WHERE id = $1`, id, money[0], money[1])
			}
			perms := AccountRepPerms
			perms.Objects = map[string]principal.ObjectGrant{
				"deal": {Read: true}, "pipeline": {Read: true}, "forecast": {Read: true}, "installation_settings": {Read: true},
			}
			if field != "" {
				perms.FieldMasks = []principal.FieldMask{{Object: "deal", Field: field, Condition: principal.MaskAlways}}
			}
			rows := askBaseSum(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), t, e).Rows

			covered := false
			for _, row := range rows {
				_, has := row[analyticsquery.CoverageColumnName]
				covered = covered || has
				if field != "" && (has || row["base"] != nil) {
					t.Errorf("a reader masked from %s was given %+v", field, row)
				}
			}
			if field == "" && !covered {
				t.Errorf("the unmasked reader got no coverage in %+v, so the masked cases prove nothing", rows)
			}
		})
	}
}
