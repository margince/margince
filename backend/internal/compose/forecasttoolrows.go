// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// forecastContributionsToTool renders the per-deal rows through the HTTP
// door's own renderer, so the tool and the endpoint return the same rows.
func forecastContributionsToTool(in []forecasting.Contribution) ([]agents.ForecastContributionResult, error) {
	encoded, err := json.Marshal(forecasting.ContributionsToWire(in))
	if err != nil {
		return nil, fmt.Errorf("compose: rendering the forecast contributions: %w", err)
	}
	out := []agents.ForecastContributionResult{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, fmt.Errorf("compose: rendering the forecast contributions: %w", err)
	}
	return out, nil
}
