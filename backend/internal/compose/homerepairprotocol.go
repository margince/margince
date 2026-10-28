// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func admitReviewedRepairActions(in crmcontracts.ActivateHomeRepairJSONBody) error {
	if in.ReviewedActions != nil && len(*in.ReviewedActions) != len(in.Review.Tasks) {
		return apperrors.ErrVersionSkew
	}
	for i, task := range in.Review.Tasks {
		if in.ReviewedActions == nil {
			if !legacyHomeRepairAction(homeRepairAction(task.Action)) {
				return apperrors.ErrVersionSkew
			}
		} else if (*in.ReviewedActions)[i] != string(task.Action) {
			return apperrors.ErrVersionSkew
		}
	}
	return nil
}

// Version-two clients know this frozen set; later actions need an explicit acknowledgement.
func legacyHomeRepairAction(action homeRepairAction) bool {
	switch action {
	case repairReclassifyMessage, repairSupersedeSuggestion, repairWithdrawProposal,
		repairArchiveReminder, repairHumanReview, repairLeaveCurrent, repairRevalidateSettlement:
		return true
	default:
		return false
	}
}
