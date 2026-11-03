// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One owner of the selection a reporting request opens on.

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The doors answer an unselected request with the sales preset. A client naming
// no metric is then asked what the Analytics screen opens on. A copy of this
// drifted once, by two metrics.
func TestTheOpeningSelectionIsTheSalesPreset(t *testing.T) {
	var sales crmcontracts.ReportingPreset
	for _, preset := range reportingPresets {
		if preset.Id == crmcontracts.ReportingPresetIdSales {
			sales = preset
		}
	}
	if len(sales.Metrics) == 0 {
		t.Fatal("the catalog publishes no sales preset, so the doors open on nothing")
	}

	opening := openingSelection()
	if !slices.Equal(opening.Metrics, sales.Metrics) {
		t.Errorf("the opening selection names %v, want the sales preset's %v",
			opening.Metrics, sales.Metrics)
	}
	if !slices.Equal(opening.Blocks, sales.Blocks) {
		t.Errorf("the opening selection draws %v, want the sales preset's %v",
			opening.Blocks, sales.Blocks)
	}
}
