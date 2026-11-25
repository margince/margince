// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/dayrule"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// Home and the team board share the tool's risk engine and its scan-depth signal.
// The service clock keeps risk facts and the rest of the day at one instant.
type attentionAtRisk struct {
	lister func(context.Context, *storekit.OwnerSet) ([]agents.SlippingDeal, bool, error)
	// tail continues the lister's candidate set past its sweeps; among asks
	// which deals that set holds.
	tail  func(context.Context, time.Time, *storekit.OwnerSet, string, int) ([]agents.SlippingDeal, []string, bool, error)
	among func(context.Context, time.Time, *storekit.OwnerSet, []ids.UUID) (map[ids.UUID]bool, error)
	pool  *pgxpool.Pool
	now   attention.Clock
}

func (a attentionAtRisk) Quiet(ctx context.Context, owners *storekit.OwnerSet) ([]attention.RiskyDeal, bool, error) {
	candidates, cut, err := a.lister(ctx, owners)
	if err != nil {
		return nil, false, err
	}
	risky, err := a.risky(ctx, candidates, owners, a.now().UTC())
	return risky, cut, err
}

// RiskAfter reads the candidates past the sweeps, drawn as Quiet draws them.
func (a attentionAtRisk) RiskAfter(
	ctx context.Context, asOf time.Time, owners *storekit.OwnerSet, after string, limit int,
) ([]attention.Positioned[attention.RiskyDeal], bool, error) {
	candidates, positions, more, err := a.tail(ctx, asOf, owners, after, limit)
	if err != nil {
		return nil, false, err
	}
	risky, err := a.risky(ctx, candidates, owners, asOf.UTC())
	if err != nil {
		return nil, false, err
	}
	out := make([]attention.Positioned[attention.RiskyDeal], len(risky))
	for i, deal := range risky {
		out[i] = attention.Positioned[attention.RiskyDeal]{Row: deal, After: positions[i]}
	}
	return out, more, nil
}

// RiskAmong answers which of these deals the tail's candidate set holds.
func (a attentionAtRisk) RiskAmong(
	ctx context.Context, asOf time.Time, owners *storekit.OwnerSet, deals []ids.UUID,
) (map[ids.UUID]bool, error) {
	return a.among(ctx, asOf, owners, deals)
}

// risky draws each candidate as the queue's at-risk deal, in order.
func (a attentionAtRisk) risky(ctx context.Context, candidates []agents.SlippingDeal, owners *storekit.OwnerSet, now time.Time) ([]attention.RiskyDeal, error) {
	if len(candidates) == 0 {
		return []attention.RiskyDeal{}, nil
	}
	cover, err := a.championCover(ctx, candidates, now)
	if err != nil {
		return nil, err
	}
	// The zone the candidates were admitted in, so quiet days and the close date
	// answer one day.
	zone, err := dayrule.ZoneForOwners(ctx, a.pool, owners)
	if err != nil {
		return nil, err
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
			WinProbability:       deal.WinProbability,
			NoChampion:           cover.noChampion(deal.DealID),
			ChampionUnknown:      cover.championUnknown(deal.DealID),
			NoNextStep:           deal.NoOpenNextStep,
		})
	}
	return risky, nil
}

// idleDaysOf counts a date-only newest activity from its own date. Counted from
// the stored noon UTC, a reader far east of UTC saw the deal a day less quiet.
func idleDaysOf(deal agents.SlippingDeal, now time.Time, zone *time.Location) int {
	since := occurredday.Moment{
		At:       idlebase.Since(deal.CreatedAt, deal.LastActivityAt),
		DateOnly: deal.LastActivityAt != nil && deal.LastActivityDateOnly,
	}
	return max(occurredday.DaysSince(since, now, zone), 0)
}
