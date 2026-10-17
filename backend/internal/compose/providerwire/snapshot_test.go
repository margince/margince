// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package providerwire

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestSnapshotRendersTheFrozenConfigurationAsTheLiveOneIsSpelled(t *testing.T) {
	t.Parallel()
	refresh, limit := 30, 50
	got := Snapshot(provider.Snapshot{
		Mode:             "on_demand",
		Preset:           "full",
		Categories:       []provider.Category{"mobile", "professional_email"},
		AutomaticCreate:  true,
		RefreshAfterDays: &refresh,
		DailyRunLimit:    &limit,
	})
	if got.Mode != crmcontracts.ProviderConnectionModeOnDemand || got.Preset != "full" || !got.AutomaticIndividualCreate || got.AutomaticImport {
		t.Errorf("snapshot = %+v, want mode on_demand, preset full, create on, import off", got)
	}
	if !got.Categories["mobile"] || !got.Categories["professional_email"] || len(got.Categories) != 2 {
		t.Errorf("categories = %v, want exactly the two frozen ones", got.Categories)
	}
	if got.RefreshAfterDays == nil || *got.RefreshAfterDays != 30 || got.DailyRunLimit == nil || *got.DailyRunLimit != 50 {
		t.Errorf("limits = %v / %v, want 30 and 50", got.RefreshAfterDays, got.DailyRunLimit)
	}
}

func TestSnapshotOfNoCategoriesIsAnEmptyMapNotNull(t *testing.T) {
	t.Parallel()
	if got := Snapshot(provider.Snapshot{}); got.Categories == nil {
		t.Error("categories is nil, which is written as null where the contract requires an object")
	}
}
