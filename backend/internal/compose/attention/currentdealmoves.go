// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sourceDealAdvice = "deal_advice"

type preparedDealMoves struct {
	moves map[ids.UUID]crmcontracts.DealStatusCardMove
}

func (s *Service) readingDealMoves(ctx context.Context, day crmcontracts.Attention) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	scoped := *s
	scoped.currentDealMoves = &preparedDealMoves{}
	if s.dealMoves == nil {
		return &scoped, nil
	}
	seen := map[ids.UUID]bool{}
	var wanted []ids.UUID
	items := append([]crmcontracts.AttentionItem{}, day.ThisMorning...)
	if day.AtRisk != nil {
		items = append(items, *day.AtRisk...)
	}
	for _, item := range items {
		if item.Subject != nil && item.Subject.Type == subjectDeal {
			id := ids.UUID(item.Subject.Id)
			if !seen[id] {
				wanted = append(wanted, id)
				seen[id] = true
			}
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
