// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const rateSourceMigration = "core/1790931616_a_price_says_where_it_came_from"

func auditRate(ctx context.Context, t *testing.T, conn *pgx.Conn, model, actorType, actorID string) {
	t.Helper()
	if _, err := conn.Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
		SELECT $2, $3, 'create', 'ai_model_rate', id, NULL, '{}'::jsonb FROM ai_model_rate WHERE model_id = $1`,
		model, actorType, actorID); err != nil {
		t.Fatalf("auditing %s: %v", model, err)
	}
}

func rateSources(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]string {
	t.Helper()
	rows, err := conn.Query(ctx, `SELECT model_id, source FROM ai_model_rate`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) ([2]string, error) {
		var pair [2]string
		return pair, r.Scan(&pair[0], &pair[1])
	})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, p := range got {
		out[p[0]] = p[1]
	}
	return out
}

// A price typed by hand carries a human on its audit trail; the seed writes
// none and the Vertex copy is the migration's, so those are the seed's.
func TestEveryExistingPriceIsFiledByWhoWroteIt(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	runFile(ctx, t, conn, rateSourceMigration+".down.sql")
	seedRate(ctx, t, conn, "gemini", "typed-by-hand", "2026-08-01", 1)
	seedRate(ctx, t, conn, "gemini", "planted-by-seed", "2026-08-01", 2)
	seedRate(ctx, t, conn, "gemini_vertex", "copied-by-migration", "2026-08-01", 3)
	auditRate(ctx, t, conn, "typed-by-hand", "human", "human:admin")
	auditRate(ctx, t, conn, "copied-by-migration", "system", "migration")

	runFile(ctx, t, conn, rateSourceMigration+".up.sql")

	got := rateSources(ctx, t, conn)
	want := map[string]string{"typed-by-hand": "manual", "planted-by-seed": "seed", "copied-by-migration": "seed"}
	for model, source := range want {
		if got[model] != source {
			t.Errorf("%s filed as %q, want %q", model, got[model], source)
		}
	}
	seedRate(ctx, t, conn, "openai", "writer-naming-nothing", "2026-08-01", 4)
	if got := rateSources(ctx, t, conn)["writer-naming-nothing"]; got != "manual" {
		t.Errorf("a writer naming no source filed %q, want manual: the sync must leave it alone", got)
	}
	if _, err := conn.Exec(ctx, `UPDATE ai_model_rate SET source = 'guess' WHERE model_id = 'typed-by-hand'`); err == nil {
		t.Error("a source outside manual/catalogue/seed was stored")
	}
}
