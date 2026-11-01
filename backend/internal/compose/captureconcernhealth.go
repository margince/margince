// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
)

func captureConcernCounts(ctx context.Context, tx pgx.Tx, now time.Time) (map[string]int, error) {
	concerns, err := capture.FleetConcernsTx(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, concern := range concerns {
		counts[concern.Connection.Concern]++
	}
	return counts, nil
}

func captureConcernCondition(kind string) crmcontracts.HealthSourceCondition {
	switch kind {
	case "reauth_required":
		return healthWaitingHuman
	case "capture_silent":
		return healthUnverified
	default:
		return healthTechnical
	}
}
