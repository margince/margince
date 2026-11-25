// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const claudeFivePrice = "core/1795621953_claude_five_models_are_priced"

// claudeRate reads the in-force input price and the long-context line of an
// Anthropic model; -1 for no row, and a nil line for a flat price.
func claudeRate(ctx context.Context, t *testing.T, conn *pgx.Conn, model string) (input int64, line *int64) {
	t.Helper()
	err := conn.QueryRow(ctx, `SELECT input_per_mtok_microusd, long_context_above_tokens FROM ai_model_rate
		WHERE provider = 'anthropic' AND model_id = $1 AND effective_date <= CURRENT_DATE
		ORDER BY effective_date DESC LIMIT 1`, model).Scan(&input, &line)
	if err == pgx.ErrNoRows {
		return -1, nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return input, line
}

// An installed sheet gains the two Claude 5.5 rows the seed carries, Haiku with
// its 100k band. A price an admin set and an empty sheet are left as they are.
func TestAnInstalledSheetGainsTheSeededClaudeFivePrices(t *testing.T) {
	dsn, _ := dsns(t)
	ctx := context.Background()
	cases := []struct {
		name                  string
		seed                  func(*pgx.Conn)
		wantSonnet, wantHaiku int64
		wantHaikuBand         int64
	}{
		{"a provisioned sheet", func(c *pgx.Conn) {
			seedRate(ctx, t, c, "gemini", "gemini-3.5-flash", "2026-08-01", 1_500_000)
		}, 2_000_000, 100_000, 100_000},
		{"a price set by hand", func(c *pgx.Conn) {
			seedRate(ctx, t, c, "gemini", "gemini-3.5-flash", "2026-08-01", 1_500_000)
			seedRate(ctx, t, c, "anthropic", "claude-sonnet-5-5", "2026-11-01", 1_800_000)
		}, 1_800_000, 100_000, 100_000},
		{"an empty sheet", func(*pgx.Conn) {}, -1, -1, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := connect(t, dsn)
			headSchema(t, conn)
			if _, err := conn.Exec(ctx, `DELETE FROM ai_model_rate`); err != nil {
				t.Fatal(err)
			}
			c.seed(conn)

			runFile(ctx, t, conn, claudeFivePrice+".up.sql")
			runFile(ctx, t, conn, claudeFivePrice+".up.sql")

			sonnet, sonnetLine := claudeRate(ctx, t, conn, "claude-sonnet-5-5")
			haiku, haikuLine := claudeRate(ctx, t, conn, "claude-haiku-5-5")
			if sonnet != c.wantSonnet || haiku != c.wantHaiku {
				t.Errorf("input prices = sonnet %d, haiku %d; want %d and %d", sonnet, haiku, c.wantSonnet, c.wantHaiku)
			}
			if sonnetLine != nil {
				t.Errorf("sonnet has a long-context line %d, want a flat price", *sonnetLine)
			}
			if c.wantHaiku > 0 && (haikuLine == nil || *haikuLine != c.wantHaikuBand) {
				t.Errorf("haiku long-context line = %v, want %d", haikuLine, c.wantHaikuBand)
			}
		})
	}
}
