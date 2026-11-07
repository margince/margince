// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Inside a read that reuses generic plans as a whole, a statement that plans
// per bind still gets a custom plan, and the outer mode comes back after it.
func TestPlanEachBindHoldsInsideWholeReadReusePoC(t *testing.T) {
	ctx := context.Background()
	pool, err := database.NewPool(ctx, os.Getenv("POC_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var ws ids.UUID
	owner, err := database.NewPool(ctx, os.Getenv("POC_OWNER_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err := owner.QueryRow(ctx, `SELECT id FROM workspace LIMIT 1`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	mode := func(ctx context.Context) string {
		var m string
		if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT current_setting('plan_cache_mode')`).Scan(&m)
		}); err != nil {
			t.Fatal(err)
		}
		return m
	}
	var outer, inner, after string
	err = database.WithWorkspaceSnapshot(principal.WithWorkspaceID(ctx, ws), pool, func(ctx context.Context) error {
		return database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			outer = mode(ctx)
			if err := database.PlanEachBind(ctx, func(ctx context.Context) error { inner = mode(ctx); return nil }); err != nil {
				return err
			}
			after = mode(ctx)
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if outer != "force_generic_plan" || inner != "force_custom_plan" || after != "force_generic_plan" {
		t.Errorf("plan modes: outer %s, per bind %s, after %s", outer, inner, after)
	}
	t.Logf("plan modes: whole read %s, per-bind statement %s, after it %s", outer, inner, after)
}
