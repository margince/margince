// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package coachdeal reads the deal a coaching notice names, for notices.
package coachdeal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/stillowed"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Reader reads the deal a next-step notice names. The deal comes
// through the deal module's own read, so the coach sees it under the grants
// GET /deals/{id} applies. Whether a next step is planned is the predicate the
// at-risk lane uses, so the notice and the recipient's queue agree.
type Reader struct {
	pool  *pgxpool.Pool
	deals *deals.Store
	now   func() time.Time
}

// Reach is what the coaching path may reach: whom the caller may coach, and
// the deals they may name.
func Reach(mates notices.Teammates, pool *pgxpool.Pool, store *deals.Store) notices.CoachReach {
	return notices.CoachReach{Mates: mates, Deals: Reader{pool: pool, deals: store, now: time.Now}}
}

// DealToCoach answers the deal's name, owner and state as the caller may read them.
func (s Reader) DealToCoach(ctx context.Context, id ids.UUID) (notices.CoachDeal, error) {
	deal, err := s.deals.GetDeal(ctx, ids.From[ids.DealKind](id), storekit.LiveOnly)
	if err != nil {
		return notices.CoachDeal{}, err
	}
	var unplanned map[ids.UUID]bool
	if err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		unplanned, err = stillowed.DealsWithoutNextStepTx(ctx, tx, []ids.UUID{id}, s.now())
		return err
	}); err != nil {
		return notices.CoachDeal{}, err
	}
	facts := notices.CoachDeal{
		Name:            deal.Name,
		Open:            deal.Status == crmcontracts.DealStatusOpen,
		NextStepPlanned: !unplanned[id],
		QuietSince:      idlebase.Since(deal.CreatedAt, deal.LastActivityAt),
	}
	if deal.OwnerId != nil {
		facts.Owner = ids.UUID(*deal.OwnerId)
	}
	return facts, nil
}
