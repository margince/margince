// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func agentInvitationPrecheck(db *database.DB) approvals.StagePrecheck {
	calendar := newSchedulingCalendar(db.Pool(), nil)
	store := activities.NewStore(db).WithSchedulingCalendar(calendar)
	return func(ctx context.Context, in approvals.StageInput) error {
		approval, err := stagedSendApproval(in)
		if err != nil {
			return err
		}
		request, err := commercialRequest[crmcontracts.MeetingInvitationRequest](approval)
		if err != nil {
			return err
		}
		host, err := defaultHost(ctx, nil)
		if err != nil {
			return err
		}
		appointment, err := store.PreviewInvitation(ctx, ids.From[ids.UserKind](host), request)
		if err != nil {
			return err
		}
		return calendar.checkAppointmentConsent(ctx, ids.From[ids.UserKind](host), appointment)
	}
}
