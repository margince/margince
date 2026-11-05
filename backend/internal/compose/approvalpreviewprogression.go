// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func stagePreviewKind(kind string) bool {
	return kind == previewAdvanceDeal || kind == previewProgressDeal || kind == deals.StageProgressionKind
}

func validateProgressionPreview(approval crmcontracts.Approval, current map[string]json.RawMessage) error {
	if approval.TargetEntityType == nil || *approval.TargetEntityType != string(recordTypeDeal) || approval.TargetEntityId == nil {
		return apperrors.ErrInvalidArgument
	}
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return err
	}
	change, err := deals.ReadProgressionChange(raw)
	if err != nil || change.DealID.UUID != ids.UUID(*approval.TargetEntityId) {
		return apperrors.ErrInvalidArgument
	}
	var stage ids.StageID
	if err := json.Unmarshal(current[fieldStageID], &stage); err != nil {
		return apperrors.ErrInvalidArgument
	}
	if stage != change.FromStageID {
		return apperrors.ErrVersionSkew
	}
	return nil
}
