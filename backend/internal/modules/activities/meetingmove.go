// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Moving a captured meeting the calendar has rescheduled.
//
// Capture writes a meeting once, under its calendar event's natural key, and a
// later pull of the same event was a replay that wrote nothing. So a meeting
// moved in the calendar kept its old date on the timeline — and one moved more
// than a year ahead, past the calendar connector's capture horizon, was no
// longer even delivered. This is the write that moves it. It belongs here for
// the reason the cancellation does: this module owns `activity`.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// auditFieldOccurredAt and auditFieldDuration are the audit image's keys for the
// two columns a move writes.
const (
	auditFieldOccurredAt = "occurred_at"
	auditFieldDuration   = "duration_seconds"
)

// MoveCapturedMeetingFor sets a captured meeting's start, length and precision
// to what the calendar now says, and reports whether anything changed. Its
// caller must prove its standing first: a connector acting on a row it found
// by the provider's event id.
//
// The same row rules as a cancellation, under the row's own lock: only a
// meeting, only a live and unrestricted one. A start the calendar could not
// state (zero), or one outside the storable range, moves nothing: the stored
// start is better than capture time.
//
// A provider issues one event id to every calendar holding the event, so a
// second connection presenting it is ordinarily the same meeting on another
// attendee's calendar — and ordinarily is not a gate: nothing in the id itself
// separates that attendee from a connection that simply stated it. holds
// (SeatStanding) is what does, decided under the row's own lock.
//
// An unchanged start and length write nothing, so a resynced calendar costs no
// audit row and no event. A whole-day move lands on its date's anchor. An
// outcome the new start has not reached goes back to booked.
func MoveCapturedMeetingFor(holds SeatStanding) func(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, slot connector.MeetingSlot,
) (bool, error) {
	return func(
		ctx context.Context, tx pgx.Tx, id ids.ActivityID, slot connector.MeetingSlot,
	) (bool, error) {
		return moveCapturedMeetingTx(ctx, tx, id, slot, holds)
	}
}

func moveCapturedMeetingTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, slot connector.MeetingSlot, holds SeatStanding,
) (bool, error) {
	if slot.Start.IsZero() || !datasource.InstantInRange(slot.Start) {
		return false, nil
	}
	var stored meetingPlace
	var holdsSlot bool
	err := tx.QueryRow(ctx, `
		SELECT a.occurred_at, a.duration_seconds, a.occurred_precision, a.meeting_status, `+occupiesHostSlotSQL+` FROM activity a
		 WHERE a.id = $1
		   AND a.kind = 'meeting' AND a.restricted_at IS NULL AND a.archived_at IS NULL
		 FOR UPDATE`, id).Scan(&stored.start, &stored.duration, &stored.precision, &stored.status, &holdsSlot)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading the meeting being moved: %w", err)
	}
	mine, err := holds(ctx, tx, id)
	if err != nil {
		return false, err
	}
	// A booked hour stays an hour: the slot guard has no date to compare.
	if !mine || (slot.AllDay && holdsSlot) {
		return false, nil
	}
	// The column keeps microseconds, so the move judges, audits and announces the start it will store.
	moved := meetingPlace{start: slot.Start.UTC().Truncate(time.Microsecond), duration: slot.Duration, precision: PrecisionInstant}
	if slot.AllDay {
		moved.start, moved.precision = occurredday.Anchor(slot.Start), PrecisionDay
	}
	if stored.start.Equal(moved.start) && sameDuration(stored.duration, moved.duration) && stored.precision == moved.precision {
		return false, nil
	}
	if moved.status, err = statusAfterMove(ctx, tx, stored.status, moved.start, moved.precision, nil); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity SET occurred_at = $2, duration_seconds = $3, occurred_precision = $4, meeting_status = $5
		 WHERE id = $1`, id, moved.start, moved.duration, moved.precision, moved.status); err != nil {
		return false, fmt.Errorf("activities: moving the captured meeting %s: %w", id, err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, stored.image(moved), moved.image(stored))
	if err != nil {
		return false, err
	}
	// The event names what the contract can name. It carries no duration, so a
	// change of length alone is in the audit row and announces nothing.
	if changed, ok := moved.changedFields(stored); ok {
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
			ChangedFields: changed,
		}); err != nil {
			return false, err
		}
	}
	return true, recordMeetingTransition(ctx, tx, meetingTransition{
		ActivityID: id, Status: changedStatus(deref(stored.status), deref(moved.status)), ScheduledStart: &moved.start,
	})
}

// meetingPlace is the columns a calendar move writes. The status moves only
// when an outcome no longer holds on the new start.
type meetingPlace struct {
	start     time.Time
	duration  *int
	precision string
	status    *string
}

// image is p's audit image; it names the status only where other differs.
func (p meetingPlace) image(other meetingPlace) map[string]any {
	image := map[string]any{
		auditFieldOccurredAt: p.start, auditFieldDuration: derefOrNil(p.duration), fieldOccurredPrecision: p.precision,
	}
	if deref(other.status) != deref(p.status) {
		image[auditFieldMeetingStatus] = deref(p.status)
	}
	return image
}

// changedFields is the event delta from before to p, false when the move
// changed only the length.
func (p meetingPlace) changedFields(before meetingPlace) (crmcontracts.PublicEventActivityChangedFields, bool) {
	var fields crmcontracts.PublicEventActivityChangedFields
	if !before.start.Equal(p.start) {
		fields.OccurredAt = &p.start
	}
	if before.precision != p.precision {
		precision := crmcontracts.PublicEventActivityChangedFieldsOccurredPrecision(p.precision)
		fields.OccurredPrecision = &precision
	}
	if status := changedStatus(deref(before.status), deref(p.status)); status != "" {
		fields.MeetingStatus = new(crmcontracts.PublicEventActivityChangedFieldsMeetingStatus(status))
	}
	return fields, fields.OccurredAt != nil || fields.OccurredPrecision != nil || fields.MeetingStatus != nil
}

func sameDuration(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
