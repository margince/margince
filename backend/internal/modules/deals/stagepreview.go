// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StageTransitionPreview projects the same fields and effects as the stage writer.
type StageTransitionPreview struct {
	Before  json.RawMessage
	After   json.RawMessage
	Effects []string
}

// PreviewAdvanceDeal derives the writer's patch without applying it. Closing times
// and currency conversions describe this read; execution resolves them again.
func (s *Store) PreviewAdvanceDeal(ctx context.Context, id ids.DealID, in AdvanceDealInput) (StageTransitionPreview, error) {
	var out StageTransitionPreview
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, "pipeline", principal.ActionRead); err != nil {
		return out, err
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, dealTable, id.UUID); err != nil {
			return err
		}
		current, err := readDeal(ctx, tx, id, storekit.LiveOnly, nil)
		if err != nil {
			return err
		}
		visible, err := finishDealForCaller(ctx, tx, current)
		if err != nil {
			return err
		}
		if current.PipelineId == nil || current.StageId == nil {
			return apperrors.ErrInvalidArgument
		}
		if err := storekit.EnsureIfMatch(ctx, tx, dealTable, id.UUID, in.IfVersion); err != nil {
			return err
		}
		if ids.UUID(*current.StageId) == in.ToStageID.UUID {
			out.Before, out.After = json.RawMessage("{}"), json.RawMessage("{}")
			return nil
		}
		semantic, _, err := resolveAdvanceTarget(ctx, tx, in.ToStageID, current)
		if err != nil {
			return err
		}
		patch, status, err := s.stageTransitionPatch(ctx, tx, current, in, semantic)
		if err != nil {
			return err
		}
		if stagePreviewWithheld(visible.MaskedFields, patch.After(), DealStatus(current.Status), DealStatus(status)) {
			return apperrors.ErrPermissionDenied
		}
		out.Effects = wonEffects(status, patch)
		out.Before, err = json.Marshal(patch.Before())
		if err != nil {
			return err
		}
		out.After, err = json.Marshal(patch.After())
		return err
	})
	return out, err
}

const (
	effectClassifyCorrespondence = "classify_correspondence"
	effectStartDelivery          = "start_project_delivery"
)

func wonEffects(status string, patch *storekit.Patch) []string {
	if DealStatus(status) != DealWon {
		return nil
	}
	effects := []string{effectClassifyCorrespondence}
	if _, changed := patch.After()[filterStatus]; changed {
		effects = append(effects, effectStartDelivery)
	}
	return effects
}

func stagePreviewWithheld(masked *[]string, changes map[string]any, before, after DealStatus) bool {
	if masked == nil {
		return false
	}
	for _, field := range *masked {
		if _, written := changes[field]; written {
			return true
		}
		switch field {
		case filterPipelineID, filterStageID, filterStatus:
			return true
		case amountField, currencyField:
			if before != DealOpen || after != DealOpen {
				return true
			}
		}
	}
	return false
}
