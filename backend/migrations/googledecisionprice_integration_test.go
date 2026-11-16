// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

const googleDecisionPrice = "core/1794796825_google_decisions_are_priced"

// decisionRate reads the gemini_decision row's input and output price and its
// lane; all zero when there is none.
func decisionRate(ctx context.Context, t *testing.T, conn *pgx.Conn) (input, output int64, lane string) {
	t.Helper()
	err := conn.QueryRow(ctx, `SELECT input_per_mtok_microusd, output_per_mtok_microusd, lane FROM ai_model_rate
		WHERE provider = 'gemini_decision' AND model_id = 'gemini-decision-1'
		ORDER BY effective_date DESC LIMIT 1`).Scan(&input, &output, &lane)
	if err != nil && err != pgx.ErrNoRows {
		t.Fatal(err)
	}
	return input, output, lane
}

// An installed sheet gains the row the seed carries; a price an admin set and
// an installation not yet provisioned are left as they are.
func TestAnInstalledSheetGainsTheSeededGoogleDecisionPrice(t *testing.T) {
	var seeded ai.ModelRate
	for _, r := range ai.SeedModelRates(clocktest.Now(t)) {
		if r.Provider == "gemini_decision" && r.ModelID == "gemini-decision-1" {
			seeded = r
		}
	}
	if seeded.Provider == "" {
		t.Fatal("the seed carries no gemini_decision row, so this compares the migration against nothing")
	}
	dsn, _ := dsns(t)
	ctx := context.Background()
	cases := []struct {
		name      string
		seed      func(*pgx.Conn)
		wantInput int64
		wantLane  string
	}{
		{"a provisioned sheet", func(c *pgx.Conn) {
			seedRate(ctx, t, c, "gemini", "gemini-3.5-flash", "2026-08-01", 1_500_000)
		}, seeded.InputPerMTokMicroUSD, string(seeded.Lane)},
		{"a price set by hand", func(c *pgx.Conn) {
			seedRate(ctx, t, c, "gemini_decision", "gemini-decision-1", "2026-11-01", 90_000)
		}, 90_000, "chat"},
		{"an empty sheet", func(*pgx.Conn) {}, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := connect(t, dsn)
			headSchema(t, conn)
			if _, err := conn.Exec(ctx, `DELETE FROM ai_model_rate`); err != nil {
				t.Fatal(err)
			}
			c.seed(conn)

			runFile(ctx, t, conn, googleDecisionPrice+".up.sql")
			runFile(ctx, t, conn, googleDecisionPrice+".up.sql")

			input, output, lane := decisionRate(ctx, t, conn)
			if input != c.wantInput || output != seeded.OutputPerMTokMicroUSD || lane != c.wantLane {
				t.Errorf("row = input %d, output %d, lane %q; want input %d, output %d, lane %q",
					input, output, lane, c.wantInput, seeded.OutputPerMTokMicroUSD, c.wantLane)
			}
		})
	}
}
