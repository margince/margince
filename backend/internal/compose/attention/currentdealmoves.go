// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sourceDealAdvice = "deal_advice"

type preparedDealMoves struct {
	moves map[ids.UUID]crmcontracts.DealStatusCardMove
}

// readingDealMoves reads the current advice for the deal rows a page shows that
// the brief or the at-risk lane raised. A deal the cut left out costs no read,
// and a deal only the backlog tail reached carries no advice.
func (s *Service) readingDealMoves(
	ctx context.Context, rows []crmcontracts.WorklistItem, day crmcontracts.Attention,
) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	defer timeStage(ctx, "deal_moves")()
	scoped := *s
	scoped.currentDealMoves = &preparedDealMoves{}
	if s.dealMoves == nil {
		return &scoped, nil
	}
	advised := laneDeals(day)
	seen := map[ids.UUID]bool{}
	var wanted []ids.UUID
	for _, row := range rows {
		if id, ok := needsDealMove(row); ok && advised[id] && !seen[id] {
			wanted = append(wanted, id)
			seen[id] = true
		}
	}
	if len(wanted) == 0 {
		return &scoped, nil
	}
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		scoped.currentDealMoves.moves, err = s.dealMoves.CurrentMoves(ctx, wanted)
		return err
	})
	if err == nil {
		return &scoped, nil
	}
	scoped.currentDealMoves.moves = nil
	reason := crmcontracts.WorklistSourceUnavailableReasonFailed
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		reason = crmcontracts.WorklistSourceUnavailableReasonWithheld
	}
	rowless := false
	note := &crmcontracts.WorklistSourceUnavailable{Source: sourceDealAdvice, Reason: reason, ContributesRows: &rowless}
	if reason == crmcontracts.WorklistSourceUnavailableReasonFailed {
		slog.Log(ctx, laneFailureLevel(ctx), "current deal advice could not be checked", "error", err)
	}
	return &scoped, note
}

// laneDeals is every deal the brief or the at-risk lane raised today.
func laneDeals(day crmcontracts.Attention) map[ids.UUID]bool {
	items := slices.Clone(day.ThisMorning)
	if day.AtRisk != nil {
		items = append(items, *day.AtRisk...)
	}
	deals := make(map[ids.UUID]bool, len(items))
	for _, item := range items {
		if item.Subject != nil && item.Subject.Type == subjectDeal {
			deals[ids.UUID(item.Subject.Id)] = true
		}
	}
	return deals
}
