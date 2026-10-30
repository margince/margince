// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
)

// Home and the team board share the tool's risk engine and its scan-depth signal.
// The service clock keeps risk facts and the rest of the day at one instant.
type attentionAtRisk struct {
	lister func(context.Context, *storekit.OwnerSet) ([]agents.SlippingDeal, bool, error)
	pool   *pgxpool.Pool
	now    attention.Clock
}

func (a attentionAtRisk) Quiet(ctx context.Context, owners *storekit.OwnerSet) ([]attention.RiskyDeal, bool, error) {
	candidates, cut, err := a.lister(ctx, owners)
	if err != nil {
		return nil, false, err
	}
	now := a.now().UTC()
	cover, err := a.championCover(ctx, candidates, now)
	if err != nil {
		return nil, false, err
	}
	var zone *time.Location
	if err := database.WithWorkspaceTx(ctx, a.pool, func(tx pgx.Tx) error {
		var zoneErr error
		zone, zoneErr = identity.ZoneAppliedTx(ctx, tx)
		return zoneErr
	}); err != nil {
		return nil, false, err
	}
	risky := make([]attention.RiskyDeal, 0, len(candidates))
	for _, deal := range candidates {
		risky = append(risky, attention.RiskyDeal{
			DealID:               deal.DealID,
			Name:                 deal.Name,
			StageID:              deal.StageID,
			OwnerID:              deal.OwnerID,
			AmountMinor:          deal.AmountMinor,
			Currency:             deal.Currency,
			QuietDays:            idleDaysOf(deal, now, zone),
			LastTouchedAt:        idlebase.Since(deal.CreatedAt, deal.LastActivityAt),
			CloseOverdue:         deal.CloseOverdue,
			ExpectedCloseDate:    deal.ExpectedCloseDate,
			CloseDateProvisional: deal.CloseDateProvisional,
			ForecastCategory:     deal.ForecastCategory,
			NoChampion:           cover.noChampion(deal.DealID),
			ChampionUnknown:      cover.championUnknown(deal.DealID),
			NoNextStep:           deal.NoOpenNextStep,
		})
	}
	return risky, cut, nil
}

func idleDaysOf(deal agents.SlippingDeal, now time.Time, zone *time.Location) int {
	return max(elapsed.Days(idlebase.Since(deal.CreatedAt, deal.LastActivityAt), now, zone), 0)
}
