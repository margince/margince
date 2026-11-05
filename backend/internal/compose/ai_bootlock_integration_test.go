// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The machinery that reads the ai settings with no request behind it: the
// binding's seeds, the watcher's task overrides and the spend meter's budget.
// Each takes the process binding, so none of these tests runs in parallel.

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bindForTest binds the api's configuration loaded from environ until the
// test ends.
func bindForTest(t *testing.T, environ ...string) {
	t.Helper()
	UseConfig(bootConfigForTest(t, config.RoleAPI, environ...))
	t.Cleanup(func() { UseConfig(nil) })
}

func routingRows(t *testing.T, e *integration.Env) int {
	t.Helper()
	var rows int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM setting WHERE key = $1`, ai.RoutingKey).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

const bootLane = `{"provider":"fake","model":"boot"}`

const bootBinding = `MARGINCE_AI_ROUTING_BINDING={"profile":"eu_hosted","tiers":{"local_small":` + bootLane +
	`,"cheap_cloud":` + bootLane + `,"premium":` + bootLane + `,"frontier":` + bootLane + `},` +
	`"embeddings":{"provider":"fake","model":"boot-embed","dimensions":8}}`

// A binding the environment sets is in force. The declared seed writes no row
// that would act only once the variable is gone, and reports nothing discarded.
func TestABootSetBindingIsNeitherSeededNorReportedDiscarded(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	bindForTest(t, bootBinding)

	var discarded []string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return seedRoutingBinding(ctx, tx, declaredFromFile(t), &discarded)
	}); err != nil {
		t.Fatalf("seeding under a boot-set binding: %v", err)
	}
	if len(discarded) != 0 {
		t.Errorf("reported %v discarded; the boot-set binding is the one in force", discarded)
	}
	if err := SeedRoutingIfUnset(context.Background(), e.Pool, declaredFromFile(t), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("the boot-time seed under a boot-set binding: %v", err)
	}
	if rows := routingRows(t, e); rows != 0 {
		t.Errorf("the declared binding wrote %d rows under a boot-set one, want none", rows)
	}
}

// The watcher publishes the overrides the environment sets, over the ones an
// admin saved before the operator set them.
func TestABootSetTaskOverrideReachesARunningRouter(t *testing.T) {
	e := integration.Setup(t)
	if rec := putTaskOverrides(overrideSeat(e), taskOverrideHandlers(e), `{"capture_classify":{"thinking":"low"}}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("saving the admin's override: %d %s", rec.Code, rec.Body)
	}
	bindForTest(t, `MARGINCE_AI_TASK_OVERRIDES={"capture_classify":{"thinking":"high"}}`)
	w, router := watcherServing(t, routingFixture(t, "serving"))
	w.pool = e.Pool

	w.refreshTaskOverrides(context.Background())

	want := ai.TaskOverrides{ai.TaskCaptureClassify: {Thinking: "high"}}.Revision()
	if got := router.TaskOverridesRevision(); got != want {
		t.Errorf("the Router serves revision %q, want the boot-set overrides' %q", got, want)
	}
}

// Spend is metered against the allowance the environment sets, over a saved
// one: the meter reads it with no request behind it.
func TestTheSpendMeterReadsABootSetAllowance(t *testing.T) {
	e := integration.Setup(t)
	saved := int64(999)
	if err := config.Save(e.Admin(), StoredConfig(e.Pool), ai.BudgetSettings,
		ai.BudgetConfig{TokensPerFullUser: 9, CompanyMonthlyTokens: &saved}); err != nil {
		t.Fatalf("saving the admin's allowance: %v", err)
	}
	bindForTest(t, `MARGINCE_AI_BUDGET={"tokens_per_full_user":5,"company_monthly_tokens":777}`)

	monthly, err := NewSeatBudget(e.Pool).MonthlyTokenBudget(context.Background(), ids.From[ids.WorkspaceKind](e.WS))
	if err != nil {
		t.Fatal(err)
	}
	if monthly != 777 {
		t.Errorf("the meter allows %d tokens, want the 777 the environment sets over the saved 999", monthly)
	}
}
