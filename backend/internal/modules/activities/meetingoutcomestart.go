// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// codeInFuture is the house code for a moment that has not come yet.
const codeInFuture = "in_future"

// MeetingNotTakenPlaceError refuses held or no_show on a meeting whose start
// has not passed.
type MeetingNotTakenPlaceError struct{}

func (e *MeetingNotTakenPlaceError) Error() string {
	return "by its date or start time this meeting has not taken place yet, so it cannot be held or a no-show; use booked or canceled, or for a meeting with only a date, send the time it started"
}

// FieldFault names meeting_status, the value the caller sent too early.
func (e *MeetingNotTakenPlaceError) FieldFault() (field, code, message string) {
	return auditFieldMeetingStatus, codeInFuture, e.Error()
}

// refuseOutcomeBeforeStart judges the status and start the patch leaves the
// meeting with, so a patch that moves a held meeting ahead is refused too.
func refuseOutcomeBeforeStart(ctx context.Context, tx pgx.Tx, current crmcontracts.Activity, in UpdateActivityInput) error {
	if current.Kind != crmcontracts.ActivityKindMeeting {
		return nil
	}
	status := in.MeetingStatus
	start, precision := current.OccurredAt, string(current.OccurredPrecision)
	if in.OccurredAt != nil {
		start = *in.OccurredAt
	}
	if in.OccurredPrecision != nil {
		precision = *in.OccurredPrecision
	}
	if status == nil && (in.OccurredAt != nil || in.OccurredPrecision != nil) && current.MeetingStatus != nil {
		status = new(string(*current.MeetingStatus))
	}
	return refuseOutcomeAt(ctx, tx, status, &start, precision, in.recordedAt)
}

// refuseLoggedOutcomeBeforeStart is the same refusal for a meeting being
// logged. An undated one starts when it is recorded, so it has taken place.
func refuseLoggedOutcomeBeforeStart(ctx context.Context, tx pgx.Tx, in LogActivityInput) error {
	if in.Kind != KindMeeting {
		return nil
	}
	return refuseOutcomeAt(ctx, tx, in.MeetingStatus, in.OccurredAt, storedPrecision(in.OccurredPrecision), in.recordedAt)
}

// refuseOutcomeAt refuses held or no_show before the meeting took place at
// asOf. Took place is occurredday's SQL rule, the one awaitingOutcomeClause asks.
func refuseOutcomeAt(ctx context.Context, tx pgx.Tx, status *string, start *time.Time, precision string, asOf *time.Time) error {
	if status == nil || (*status != string(crmcontracts.ActivityMeetingStatusHeld) &&
		*status != string(crmcontracts.ActivityMeetingStatusNoShow)) {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	at := sprintf("coalesce($%d::timestamptz, now())", arg(asOf))
	moment := sprintf(`SELECT coalesce($%d::timestamptz, %s) AS occurred_at, $%d::text AS occurred_precision`, arg(start), at, arg(precision))
	var took bool
	if err := tx.QueryRow(ctx, `SELECT `+occurredday.MeetingTookPlaceSQL("m", at)+` FROM (`+moment+`) m`, args...).Scan(&took); err != nil {
		return fmt.Errorf("activities: reading whether the meeting took place: %w", err)
	}
	if !took {
		return &MeetingNotTakenPlaceError{}
	}
	return nil
}

// ProbeLoggedOutcome asks, without writing, whether logging in would be
// refused for an outcome its meeting has not reached. An import's dry run asks.
func (s *Store) ProbeLoggedOutcome(ctx context.Context, in LogActivityInput) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	if err := admitLoggedPrecision(&in); err != nil {
		return err
	}
	at := s.now()
	in.recordedAt = &at
	return s.tx(ctx, func(tx pgx.Tx) error { return refuseLoggedOutcomeBeforeStart(ctx, tx, in) })
}

// ProbeOutcome is ProbeLoggedOutcome for a patch of the activity id.
func (s *Store) ProbeOutcome(ctx context.Context, id ids.ActivityID, in UpdateActivityInput) error {
	at := s.now()
	in.recordedAt = &at
	return s.tx(ctx, func(tx pgx.Tx) error {
		current, err := readActivity(ctx, tx, id, storekit.LiveOnly)
		if err == nil {
			err = admitPatchPrecision(ctx, tx, current, &in)
		}
		if err != nil {
			return err
		}
		return refuseOutcomeBeforeStart(ctx, tx, current, in)
	})
}
