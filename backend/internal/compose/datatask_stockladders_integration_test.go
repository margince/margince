// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// storeGeminiRouting saves gemini_cloud's binding with no task order of its
// own, as the setup screen bound it before presets ordered tasks. customise
// rebinds premium to the cheap model, as an admin might.
func storeGeminiRouting(t *testing.T, e *integration.Env, routing *ai.RoutingStore, customise bool) {
	t.Helper()
	storePresetRouting(t, e, routing, "gemini_cloud.yaml", customise)
}

// storePresetRouting saves a committed preset's binding with no task order.
func storePresetRouting(t *testing.T, e *integration.Env, routing *ai.RoutingStore, preset string, customise bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "presets", preset))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ai.ParsePreset(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg.TaskLadders = nil
	if customise {
		premium := cfg.Tiers[ai.TierPremium]
		premium.Model = "gemini-3.1-flash-lite"
		cfg.Tiers[ai.TierPremium] = premium
	}
	if _, err := routing.ReplaceIfVersion(e.Admin(), cfg, ""); err != nil {
		t.Fatalf("saving the binding: %v", err)
	}
}

func routingAuditsByTheTask(t *testing.T, e *integration.Env) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='ai_routing' AND actor_id=$1`, datatask.ActorID(stockLaddersDataTask(nil).ID))
}

// A stock Gemini install takes the order its preset now declares, audited as a
// settings change; running the task again changes nothing.
func TestTheStockLaddersTaskGivesAStockInstallItsPresetsOrder(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	routing := ai.NewRoutingStore(NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	storeGeminiRouting(t, e, routing, false)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	for range 2 {
		e.RunDataTask(ctx, t, stockLaddersDataTask(routing.AdoptStockTaskLaddersTx))
	}
	stored, err := routing.Get(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []ai.Task{ai.TaskAgentLoop, ai.TaskNlSearch, ai.TaskVoiceBuild} {
		if got := stored.Ladder(task); !slices.Equal(got, []ai.Tier{ai.TierPremium, ai.TierCheapCloud}) {
			t.Errorf("%s walks %v after the task, want premium first", task, got)
		}
	}
	if got := routingAuditsByTheTask(t, e); got != 1 {
		t.Errorf("the task audited %d routing writes over two runs, want one", got)
	}
}

// An install whose admin chose their own binding keeps the contract order and
// is not written to.
func TestTheStockLaddersTaskLeavesACustomisedInstallAlone(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	routing := ai.NewRoutingStore(NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	storeGeminiRouting(t, e, routing, true)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.RunDataTask(ctx, t, stockLaddersDataTask(routing.AdoptStockTaskLaddersTx))
	stored, err := routing.Get(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []ai.Task{ai.TaskAgentLoop, ai.TaskNlSearch, ai.TaskVoiceBuild} {
		if got := stored.Ladder(task); !slices.Equal(got, ai.TaskLadder(task)) {
			t.Errorf("a customised install walks %s %v, want its contract order", task, got)
		}
	}
	if got := routingAuditsByTheTask(t, e); got != 0 {
		t.Errorf("the task wrote a customised install's routing %d time(s)", got)
	}
}

// An install bound to a preset that orders no task is not written to: no save,
// no audit row.
func TestTheStockLaddersTaskWritesNothingForAPresetThatOrdersNoTask(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	routing := ai.NewRoutingStore(NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	storePresetRouting(t, e, routing, "openrouter_cloud.yaml", false)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.RunDataTask(ctx, t, stockLaddersDataTask(routing.AdoptStockTaskLaddersTx))
	if got := routingAuditsByTheTask(t, e); got != 0 {
		t.Errorf("the task wrote an openrouter_cloud install's routing %d time(s)", got)
	}
}
