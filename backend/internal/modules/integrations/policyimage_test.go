// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"testing"
	"time"
)

// The order a request lists categories and pools in says nothing about the
// policy, and neither does a balance the provider reported since.
func TestAPolicyIsTheSameWhateverOrderAndBalanceItCarries(t *testing.T) {
	ceiling, pause, balance := 100, 10, 55
	readAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	stored := currentConfig{
		Mode: "on_demand", Preset: "basic", Categories: []string{"email", "phone"},
		Budgets: []PoolBudget{
			{Pool: "a", MonthlyCeiling: &ceiling, PauseBelowBalance: &pause, LastKnownBalance: &balance, BalanceReadAt: &readAt},
			{Pool: "b"},
		},
	}
	sent := currentConfig{
		Mode: "on_demand", Preset: "basic", Categories: []string{"phone", "email"},
		Budgets: []PoolBudget{{Pool: "a", MonthlyCeiling: &ceiling, PauseBelowBalance: &pause}},
	}
	if !configurationUnchanged(stored, sent) {
		t.Error("the same policy in another order, without the reported balance, reads as changed")
	}
}

func TestAPolicyThatMovesASettingIsChanged(t *testing.T) {
	ceiling, raised := 100, 200
	stored := currentConfig{Mode: "on_demand", Budgets: []PoolBudget{{Pool: "a", MonthlyCeiling: &ceiling}}}
	for name, merged := range map[string]currentConfig{
		"mode":    {Mode: "automatic_on_create", Budgets: stored.Budgets},
		"ceiling": {Mode: "on_demand", Budgets: []PoolBudget{{Pool: "a", MonthlyCeiling: &raised}}},
		"pool":    {Mode: "on_demand", Budgets: []PoolBudget{{Pool: "b", MonthlyCeiling: &ceiling}}},
	} {
		if configurationUnchanged(stored, merged) {
			t.Errorf("a changed %s reads as unchanged", name)
		}
	}
}
