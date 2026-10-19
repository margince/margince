// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func changedScheduledContext(ctx context.Context, tx pgx.Tx, in SendEmailInput, created time.Time) (string, error) {
	if in.Context != commsauthz.CategoryRequestedFollowup || in.Evidence.ActivityID.IsZero() {
		return "", nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return "", err
	}
	source, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](in.Evidence.ActivityID), storekit.LiveOnly)
	if err != nil {
		return "", err
	}
	if reason := changedFollowup(source, created); reason != "" {
		return reason, nil
	}
	settlement, err := settlementImage(ctx, tx, in.Evidence.ActivityID)
	if err != nil || settlement == nil {
		return "", err
	}
	verdict, known := settlement[settlementVerdictCol].(string)
	if !known || verdict != RequestSettled {
		return "", nil
	}
	through, known := settlement[settlementThroughCol].(string)
	if !known {
		return "", nil
	}
	answerID, err := ids.Parse(through)
	if err != nil {
		return "", err
	}
	answer, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](answerID), storekit.LiveOnly)
	if err != nil {
		return "", err
	}
	if answer.CreatedAt.After(created) {
		return HeldFollowupSettled, nil
	}
	return "", nil
}

func changedFollowup(source crmcontracts.Activity, created time.Time) string {
	if !source.UpdatedAt.After(created) {
		return ""
	}
	if source.MeetingStatus != nil && *source.MeetingStatus == crmcontracts.ActivityMeetingStatusCanceled {
		return HeldMeetingCanceled
	}
	if source.Kind == crmcontracts.ActivityKindTask && source.IsDone != nil && *source.IsDone {
		return HeldFollowupSettled
	}
	return ""
}
