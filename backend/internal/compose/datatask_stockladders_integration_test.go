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
// own, as an install the setup screen bound before presets ordered tasks;
// customise rebinds premium to the cheap model, as an admin might.
func storeGeminiRouting(t *testing.T, e *integration.Env, routing *ai.RoutingStore, customise bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "presets", "gemini_cloud.yaml"))
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
	return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='ai_routing' AND actor_id=$1`, datatask.ActorID(stockLaddersTaskID))
}

// A stock Gemini install takes the order its preset now declares, audited as a
// settings change; running the task again changes nothing.
func TestTheStockLaddersTaskGivesAStockInstallItsPresetsOrder(t *testing.T) {
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
	if got := stored.Ladder(ai.TaskAgentLoop); !slices.Equal(got, []ai.Tier{ai.TierPremium, ai.TierCheapCloud}) {
		t.Errorf("agent_loop walks %v after the task, want premium first", got)
	}
	if got := routingAuditsByTheTask(t, e); got != 1 {
		t.Errorf("the task audited %d routing writes over two runs, want one", got)
	}
}

// An install whose admin chose their own binding keeps the contract order and
// is not written to.
func TestTheStockLaddersTaskLeavesACustomisedInstallAlone(t *testing.T) {
	e := integration.Setup(t)
	routing := ai.NewRoutingStore(NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	storeGeminiRouting(t, e, routing, true)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.RunDataTask(ctx, t, stockLaddersDataTask(routing.AdoptStockTaskLaddersTx))
	stored, err := routing.Get(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Ladder(ai.TaskAgentLoop); !slices.Equal(got, ai.TaskLadder(ai.TaskAgentLoop)) {
		t.Errorf("a customised install walks agent_loop %v, want its contract order", got)
	}
	if got := routingAuditsByTheTask(t, e); got != 0 {
		t.Errorf("the task wrote a customised install's routing %d time(s)", got)
	}
}
