// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// retiredStageCardsDataTask expires the stage-move cards still waiting. No
// effect applies them any more; Margince moves the deal itself.
func retiredStageCardsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793883852_retired_stage_cards_expired", Version: 1,
		Title: "Expiring stage-move cards Margince no longer asks", Requires: []string{"core/1793883852"},
		Steps: []datatask.Step{{
			Name: "expire_retired_stage_cards", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: approvals.EstimateRetiredStageCards, Batch: approvals.ExpireRetiredStageCardsTx,
		}},
	}
}
