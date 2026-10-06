// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

const vertexBackfill = "core/1790878288_gemini_on_vertex_takes_a_copy_of_geminis_prices"

func seedRate(ctx context.Context, t *testing.T, conn *pgx.Conn, provider, model, day string, input int64) {
	t.Helper()
	if _, err := conn.Exec(ctx, `
		INSERT INTO ai_model_rate (provider, model_id, input_per_mtok_microusd, output_per_mtok_microusd, effective_date)
		VALUES ($1, $2, $3, 0, $4)`, provider, model, input, day); err != nil {
		t.Fatalf("seeding %s/%s: %v", provider, model, err)
	}
}

// seedFullRate plants a Gemini row with every price bucket and lane distinct,
// so a copy that drops or swaps a column cannot match it by accident.
func seedFullRate(ctx context.Context, t *testing.T, conn *pgx.Conn, model, lane, day string) {
	t.Helper()
	if _, err := conn.Exec(ctx, `
		INSERT INTO ai_model_rate (provider, model_id, lane, input_per_mtok_microusd, output_per_mtok_microusd,
		                           cache_read_per_mtok_microusd, cache_write_per_mtok_microusd, effective_date)
		VALUES ('gemini', $1, $2, 11, 22, 33, 44, $3)`, model, lane, day); err != nil {
		t.Fatalf("seeding gemini/%s: %v", model, err)
	}
}

func runFile(ctx context.Context, t *testing.T, conn *pgx.Conn, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func vertexRates(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]int64 {
	t.Helper()
	rows, err := conn.Query(ctx, `SELECT model_id || '@' || effective_date, input_per_mtok_microusd FROM ai_model_rate WHERE provider = 'gemini_vertex'`)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		got[k] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

// An installation whose Vertex sheet is empty takes a copy of every Gemini
// row, audited; running it again copies nothing more, and rolling it back
// removes only the copies nobody has touched.
func TestAnEmptyVertexSheetTakesACopyOfGeminisPricesOnce(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `DELETE FROM ai_model_rate`); err != nil {
		t.Fatal(err)
	}
	seedRate(ctx, t, conn, "gemini", "gemini-2.5-flash", "2026-08-01", 300_000)
	seedRate(ctx, t, conn, "gemini", "gemini-2.5-flash", "2026-10-01", 350_000)
	seedRate(ctx, t, conn, "openai", "gpt-5-mini", "2026-08-01", 750_000)

	runFile(ctx, t, conn, vertexBackfill+".up.sql")
	runFile(ctx, t, conn, vertexBackfill+".up.sql")

	got := vertexRates(ctx, t, conn)
	want := map[string]int64{"gemini-2.5-flash@2026-08-01": 300_000, "gemini-2.5-flash@2026-10-01": 350_000}
	if len(got) != len(want) || got["gemini-2.5-flash@2026-08-01"] != 300_000 || got["gemini-2.5-flash@2026-10-01"] != 350_000 {
		t.Errorf("Vertex rows after the upgrade = %v, want %v", got, want)
	}
	var audited int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'ai_model_rate' AND after->>'copied_from' = 'gemini'`).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 2 {
		t.Errorf("%d copies audited, want 2 (a second run copies nothing)", audited)
	}

	seedRate(ctx, t, conn, "gemini_vertex", "gemini-3.5-flash", "2026-08-01", 1_600_000)
	runFile(ctx, t, conn, vertexBackfill+".down.sql")
	if got := vertexRates(ctx, t, conn); len(got) != 1 || got["gemini-3.5-flash@2026-08-01"] != 1_600_000 {
		t.Errorf("Vertex rows after rollback = %v, want only the price an admin wrote", got)
	}
}

// A copy carries the lane and all four price buckets, and its audit row
// records the price the upgrade wrote.
func TestACopiedVertexRowCarriesEveryPriceColumn(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `DELETE FROM ai_model_rate`); err != nil {
		t.Fatal(err)
	}
	seedFullRate(ctx, t, conn, "gemini-embedding-001", "embeddings", "2026-08-01")

	runFile(ctx, t, conn, vertexBackfill+".up.sql")

	var row, audited string
	if err := conn.QueryRow(ctx, `
		SELECT concat_ws(',', lane, input_per_mtok_microusd, output_per_mtok_microusd,
		                 cache_read_per_mtok_microusd, cache_write_per_mtok_microusd)
		  FROM ai_model_rate WHERE provider = 'gemini_vertex'`).Scan(&row); err != nil {
		t.Fatal(err)
	}
	if row != "embeddings,11,22,33,44" {
		t.Errorf("copied row = %s, want embeddings,11,22,33,44", row)
	}
	if err := conn.QueryRow(ctx, `
		SELECT concat_ws(',', after->>'input_per_mtok_microusd', after->>'output_per_mtok_microusd',
		                 after->>'cache_read_per_mtok_microusd', after->>'cache_write_per_mtok_microusd')
		  FROM audit_log WHERE after->>'copied_from' = 'gemini'`).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != "11,22,33,44" {
		t.Errorf("audited prices = %s, want 11,22,33,44", audited)
	}
}

// A copy an administrator edited after the upgrade is theirs: rolling back
// keeps it, whatever order the two audit ids happen to sort in.
func TestRollingBackKeepsACopyEditedSince(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `DELETE FROM ai_model_rate`); err != nil {
		t.Fatal(err)
	}
	seedRate(ctx, t, conn, "gemini", "gemini-2.5-flash", "2026-08-01", 300_000)
	runFile(ctx, t, conn, vertexBackfill+".up.sql")
	// The edit's audit id sorts BELOW the migration's, as a same-millisecond
	// id from another backend may.
	if _, err := conn.Exec(ctx, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before, after)
		SELECT '00000000-0000-7000-8000-000000000000', 'human', 'admin', 'update', 'ai_model_rate', id, NULL, '{}'::jsonb
		  FROM ai_model_rate WHERE provider = 'gemini_vertex'`); err != nil {
		t.Fatal(err)
	}

	runFile(ctx, t, conn, vertexBackfill+".down.sql")

	if got := vertexRates(ctx, t, conn); len(got) != 1 {
		t.Errorf("Vertex rows after rollback = %v, want the edited copy kept", got)
	}
}

// An installation that already prices Vertex keeps exactly what it has.
func TestAVertexSheetWithPricesIsLeftAlone(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `DELETE FROM ai_model_rate`); err != nil {
		t.Fatal(err)
	}
	seedRate(ctx, t, conn, "gemini", "gemini-2.5-flash", "2026-08-01", 300_000)
	seedRate(ctx, t, conn, "gemini_vertex", "gemini-3.5-flash", "2026-08-01", 1_600_000)

	runFile(ctx, t, conn, vertexBackfill+".up.sql")

	if got := vertexRates(ctx, t, conn); len(got) != 1 {
		t.Errorf("Vertex rows = %v, want the one it already had", got)
	}
}

// A provisioned installation's seed already prices three Vertex models, so the
// upgrade copies nothing there; its other models take Gemini's price through
// the live fallback instead.
func TestASeededVertexSheetTakesNoCopy(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `DELETE FROM ai_model_rate`); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"gemini-3.5-flash", "gemini-3.1-flash-lite", "gemini-2.5-pro"} {
		seedRate(ctx, t, conn, "gemini", model, "2026-08-01", 1_000_000)
	}
	seedRate(ctx, t, conn, "gemini_vertex", "gemini-3.5-flash", "2026-08-01", 1_500_000)
	seedRate(ctx, t, conn, "gemini_vertex", "gemini-3.1-flash-lite", "2026-08-01", 250_000)

	runFile(ctx, t, conn, vertexBackfill+".up.sql")

	if got := vertexRates(ctx, t, conn); len(got) != 2 {
		t.Errorf("Vertex rows = %v, want only the two the seed planted", got)
	}
}
