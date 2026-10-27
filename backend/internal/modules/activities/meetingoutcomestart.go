// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// codeInFuture is the house code for a moment that has not come yet.
const codeInFuture = "in_future"

// MeetingNotTakenPlaceError refuses held or no_show on a meeting whose start
// has not passed.
type MeetingNotTakenPlaceError struct{}

func (e *MeetingNotTakenPlaceError) Error() string {
	return "a meeting that has not taken place yet cannot be held or a no-show; cancel it, or record how it went once it has started"
}

// FieldFault names meeting_status, the value the caller sent too early.
func (e *MeetingNotTakenPlaceError) FieldFault() (field, code, message string) {
	return auditFieldMeetingStatus, codeInFuture, e.Error()
}

// refuseOutcomeBeforeStart judges the start the patch leaves the meeting with,
// so moving a meeting into the past and saying it was held is one patch. Took
// place is occurredday's SQL rule, the one awaitingOutcomeClause asks.
func refuseOutcomeBeforeStart(ctx context.Context, tx pgx.Tx, current crmcontracts.Activity, in UpdateActivityInput) error {
	if !in.askedByCaller || in.recordedAt == nil || in.MeetingStatus == nil || current.Kind != crmcontracts.ActivityKindMeeting {
		return nil
	}
	if status := crmcontracts.ActivityMeetingStatus(*in.MeetingStatus); status != crmcontracts.ActivityMeetingStatusHeld &&
		status != crmcontracts.ActivityMeetingStatusNoShow {
		return nil
	}
	start, precision := current.OccurredAt, string(current.OccurredPrecision)
	if in.OccurredAt != nil {
		start = *in.OccurredAt
	}
	if in.OccurredPrecision != nil {
		precision = *in.OccurredPrecision
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	moment := sprintf(`SELECT $%d::timestamptz AS occurred_at, $%d::text AS occurred_precision`, arg(start), arg(precision))
	tookPlace := occurredday.MeetingTookPlaceSQL("m", sprintf("$%d::timestamptz", arg(*in.recordedAt)))
	var took bool
	if err := tx.QueryRow(ctx, `SELECT `+tookPlace+` FROM (`+moment+`) m`, args...).Scan(&took); err != nil {
		return fmt.Errorf("activities: reading whether the meeting took place: %w", err)
	}
	if !took {
		return &MeetingNotTakenPlaceError{}
	}
	return nil
}
