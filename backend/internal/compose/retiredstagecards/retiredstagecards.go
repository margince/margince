// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package retiredstagecards is the data task that expires the stage-move cards
// still waiting after Margince began moving the deal itself.
package retiredstagecards

import (
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask expires the stage-move cards still waiting. No
// effect applies them any more; Margince moves the deal itself.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793883852_retired_stage_cards_expired", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1793883852_retired_stage_cards_expired"), Requires: []string{"core/1793883852"},
		Steps: []datatask.Step{{
			Name: "expire_retired_stage_cards", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: approvals.EstimateRetiredStageCards, Batch: approvals.ExpireRetiredStageCardsTx,
			EmptyWhenDone: true,
		}},
	}
}
