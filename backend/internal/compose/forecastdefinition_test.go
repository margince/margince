// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// New signal rules change the learned effects and so the weighted readings. A
// movement across them must read as a definition change, not as deals moving.
func TestASignalRulesChangeIsAReadingDefinitionChange(t *testing.T) {
	t.Parallel()
	// Each signal rules version, with the reading definition that first used it.
	firstDefinition := map[string]string{"v1": "forecast_v3", "v2": "forecast_v4"}
	want, listed := firstDefinition[assurance.RiskSignalsVersion]
	if !listed || want != forecasting.DefinitionVersion {
		t.Fatalf("signal rules %s run under reading definition %s, want %q: bump forecasting.DefinitionVersion with them and list the pair here",
			assurance.RiskSignalsVersion, forecasting.DefinitionVersion, want)
	}
}
