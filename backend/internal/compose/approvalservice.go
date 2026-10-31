// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
)

// NewApprovalsService binds decisions to current context and responsibility checks.
func NewApprovalsService(db *database.DB) *approvals.Service {
	preview := newApprovalPreviewReader(db)
	return approvals.NewService(db).
		WithAssigneeCheck(activities.SeatCanHoldAutomaticWork).
		WithPreview(preview.read).
		WithContextPin(operationPromoteLead, promotionContextPin(db))
}
