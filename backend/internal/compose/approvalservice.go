// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
)

func newApprovalsService(db *database.DB) *approvals.Service {
	return approvals.NewService(db).WithAssigneeCheck(activities.SeatCanHoldAutomaticWork)
}
