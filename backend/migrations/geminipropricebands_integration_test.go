// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const proBands = "core/1793769685_gemini_pro_is_priced_in_two_bands"

// bandOf reads the band line of the provider's pro-preview row, -1 for no row
// and 0 for a row with no band.
func bandOf(ctx context.Context, t *testing.T, conn *pgx.Conn, provider string) int64 {
	t.Helper()
	var line *int64
	err := conn.QueryRow(ctx, `SELECT long_context_above_tokens FROM ai_model_rate
		WHERE provider = $1 AND model_id = 'gemini-3.1-pro-preview'
		ORDER BY effective_date DESC LIMIT 1`, provider).Scan(&line)
	if err == pgx.ErrNoRows {
		return -1
	}
	if err != nil {
		t.Fatal(err)
	}
	if line == nil {
		return 0
	}
	return *line
}

func seedProRow(ctx context.Context, t *testing.T, conn *pgx.Conn, provider string, input int64, source string) {
	t.Helper()
	if _, err := conn.Exec(ctx, `
		INSERT INTO ai_model_rate (provider, model_id, input_per_mtok_microusd, output_per_mtok_microusd,
		                           cache_read_per_mtok_microusd, effective_date, source)
		VALUES ($1, 'gemini-3.1-pro-preview', $2, 12000000, 200000, '2026-09-01', $3)`, provider, input, source); err != nil {
		t.Fatal(err)
	}
}

// An installation with no row gets the banded one. A row at the old flat price
// gains the band. A price somebody set stays as it was.
func TestGeminiProGetsItsBandOnlyWhereNobodyPricedItByHand(t *testing.T) {
	dsn, _ := dsns(t)
	ctx := context.Background()
	cases := []struct {
		name string
		seed func(*pgx.Conn)
		want int64
	}{
		{"no row", func(*pgx.Conn) {}, 200_000},
		{"the flat catalogue row", func(c *pgx.Conn) { seedProRow(ctx, t, c, "gemini", 2_000_000, "catalogue") }, 200_000},
		{"a custom row", func(c *pgx.Conn) { seedProRow(ctx, t, c, "gemini", 2_500_000, "manual") }, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := connect(t, dsn)
			headSchema(t, conn)
			if _, err := conn.Exec(ctx, `DELETE FROM ai_model_rate`); err != nil {
				t.Fatal(err)
			}
			seedRate(ctx, t, conn, "gemini", "gemini-3.5-flash", "2026-08-01", 1_500_000)
			c.seed(conn)

			runFile(ctx, t, conn, proBands+".up.sql")
			runFile(ctx, t, conn, proBands+".up.sql")

			if got := bandOf(ctx, t, conn, "gemini"); got != c.want {
				t.Errorf("gemini's pro-preview band starts at %d, want %d", got, c.want)
			}
			if got := bandOf(ctx, t, conn, "gemini_vertex"); got != -1 {
				t.Errorf("an installation that prices nothing on Vertex got a Vertex row (band %d)", got)
			}
		})
	}
}
