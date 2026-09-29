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
)

// auditFieldOccurredAt and auditFieldDuration are the audit image's keys for the
// two columns a move writes.
const (
	auditFieldOccurredAt = "occurred_at"
	auditFieldDuration   = "duration_seconds"
)

// MoveCapturedMeetingFor sets a captured meeting's start and length to what the
// calendar now says, and reports whether anything changed — for a caller that
// must prove its standing first: a connector acting on a row it found by the
// provider's event id.
//
// The same row rules as a cancellation, under the row's own lock: only a
// meeting, only a live and unrestricted one. A start the calendar could not
// state (zero) moves nothing — the stored start is better than capture time.
//
// A provider issues one event id to every calendar holding the event, so a
// second connection presenting it is ordinarily the same meeting on another
// attendee's calendar — and ordinarily is not a gate: nothing in the id itself
// separates that attendee from a connection that simply stated it. holds
// (SeatStanding) is what does, decided under the row's own lock.
//
// An unchanged start and length write nothing, so a resynced calendar costs no
// audit row and no event.
func MoveCapturedMeetingFor(holds SeatStanding) func(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, start time.Time, duration *int,
) (bool, error) {
	return func(
		ctx context.Context, tx pgx.Tx, id ids.ActivityID, start time.Time, duration *int,
	) (bool, error) {
		return moveCapturedMeetingTx(ctx, tx, id, start, duration, holds)
	}
}

func moveCapturedMeetingTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, start time.Time, duration *int, holds SeatStanding,
) (bool, error) {
	if start.IsZero() {
		return false, nil
	}
	var storedStart time.Time
	var storedDuration *int
	err := tx.QueryRow(ctx, `
		SELECT occurred_at, duration_seconds FROM activity
		 WHERE id = $1
		   AND kind = 'meeting' AND restricted_at IS NULL AND archived_at IS NULL
		 FOR UPDATE`, id).Scan(&storedStart, &storedDuration)
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
	if !mine {
		return false, nil
	}
	start = start.UTC()
	if storedStart.Equal(start) && sameDuration(storedDuration, duration) {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity SET occurred_at = $2, duration_seconds = $3
		 WHERE id = $1`, id, start, duration); err != nil {
		return false, fmt.Errorf("activities: moving the captured meeting %s: %w", id, err)
	}
	before := map[string]any{auditFieldOccurredAt: storedStart, auditFieldDuration: derefOrNil(storedDuration)}
	after := map[string]any{auditFieldOccurredAt: start, auditFieldDuration: derefOrNil(duration)}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, before, after)
	if err != nil {
		return false, err
	}
	// The event names what the contract can name. It carries occurred_at and
	// no duration, so a change of length alone is in the audit row and
	// announces nothing rather than claiming the start moved.
	if !storedStart.Equal(start) {
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
			ChangedFields: crmcontracts.PublicEventActivityChangedFields{OccurredAt: &start},
		}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func sameDuration(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
