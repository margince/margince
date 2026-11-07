// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
)

func graphProjectionSnapshot(ctx context.Context, t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	projection := make(map[string]string)
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		// A rebuild replaces maintenance state; every derived fact must agree.
		const facts = `to_jsonb(e)-ARRAY['computed_at','read_snapshot','requested_generation','processed_generation','deferred_run_id']::text[]`
		for _, table := range []string{"graph_contact_edge", "graph_interaction_edge"} {
			var rows string
			query := `SELECT COALESCE(jsonb_agg(` + facts + ` ORDER BY ` + facts + `),'[]')::text FROM ` + pgx.Identifier{table}.Sanitize() + ` e`
			if err := tx.QueryRow(ctx, query).Scan(&rows); err != nil {
				return err
			}
			projection[table] = rows
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}
