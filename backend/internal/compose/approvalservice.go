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
	store, gate := preview.activities.WithRecipientFiler(newContactsStore(db)), consentGateOn(db)
	return approvals.NewService(db).
		WithAssigneeCheck(activities.SeatCanHoldAutomaticWork).
		WithPreview(preview.read).
		WithPlanSight(approvals.PlanSight{Footprint: preview.footprint, Clears: preview.viewerClears}).
		WithContextPin(operationPromoteLead, promotionContextPin(db)).
		WithEffectActor(NewApprovalEffectActor(db)).
		WithStageAdmission(stageCallAdmission).
		WithStagePrecheck(previewSendEmail, agentMailPrecheck(store, gate)).
		WithStagePrecheck(previewSendCompanyEmail, agentMailPrecheck(store, gate)).
		WithStagePrecheck(operationSendMessage, agentChannelPrecheck(store, gate)).
		WithStagePrecheck(operationInviteMeeting, agentInvitationPrecheck(db))
}
