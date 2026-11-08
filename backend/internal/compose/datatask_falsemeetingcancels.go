// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Older calendar code cancelled a shared meeting when one attendee declined or
// removed their own copy. The evidence for reopening is who cancelled it and
// the stored original read under today's rules. A status a human or an agent
// set stands, and so does anything the evidence does not explain.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// falseCancellationLookback is how far back a meeting may lie and still be
// reopened. An older one would only put stale work in the outcome lane.
const falseCancellationLookback = 30 * 24 * time.Hour

// falseMeetingCancelsDataTask reopens meetings a calendar cancelled on behalf
// of a seat that does not organize them.
func falseMeetingCancelsDataTask(now func() time.Time) datatask.Task {
	since := func() time.Time { return now().Add(-falseCancellationLookback) }
	return datatask.Task{
		ID: "core/1794138928_calendar_cancellations_a_guest_caused", Version: 1,
		Title: "Reopening meetings a guest's calendar cancelled for everybody",
		Steps: []datatask.Step{{
			Name: "reopen_guest_cancellations", Limit: 100, MinLimit: 1,
			Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				tx, ok := q.(pgx.Tx)
				if !ok {
					return 0, false, errors.New("false cancellation estimate: the executor passed no transaction")
				}
				count, err := activities.CountCalendarCancellationsTx(ctx, tx, since(), maximum+1)
				return min(count, maximum), count > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return reopenFalseCancellationsBatch(ctx, tx, in, since())
			},
		}},
	}
}

func reopenFalseCancellationsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, since time.Time) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	found, err := activities.CalendarCancellationsAfterTx(ctx, tx, since, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(found))
	reopened := 0
	for _, c := range found {
		scanned = append(scanned, c.ID)
		guest, err := cancelledForAGuestTx(ctx, tx, c)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if !guest {
			continue
		}
		done, err := activities.ReopenCalendarCancellationTx(ctx, tx, c, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if done {
			reopened++
		}
	}
	return datatask.AdvanceUUID(in, scanned, reopened)
}

// cancelledForAGuestTx answers whether the evidence shows a guest's calendar
// cancelled the meeting rather than its organizer.
func cancelledForAGuestTx(ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation) (bool, error) {
	switch {
	case c.ActorType == string(principal.PrincipalConnector) && c.Seat != nil:
		organizer, err := capture.SeatOrganizesMeetingTx(ctx, tx, ids.From[ids.ActivityKind](c.ID), *c.Seat)
		if err != nil || organizer {
			return false, err
		}
		var owner string
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT account_label FROM capture_connection
 WHERE provider = $1 AND user_id = $2 LIMIT 1), '')`, c.Key.SourceSystem, *c.Seat).Scan(&owner); err != nil {
			return false, wrapFalseCancellation(err)
		}
		// The seat's calendar already showed whose cancellation it was, so an
		// original that cannot be read leaves the meeting to reopen.
		settlement, readable, err := storedSettlementTx(ctx, tx, c, owner)
		return !readable || settlement != meetingmap.SettleCancel, err
	case c.ActorType == string(principal.PrincipalSystem):
		var closed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity_meeting_rsvp_backfill
 WHERE activity_id = $1 AND outcome = $2)`, c.ID, rsvpBackfillClosed).Scan(&closed); err != nil || !closed {
			return false, wrapFalseCancellation(err)
		}
		var owner string
		if err := tx.QueryRow(ctx, `SELECT `+stampedMailboxLabelSQL+` FROM activity a
 WHERE a.id = $1 AND a.archived_at IS NULL AND a.restricted_at IS NULL`, c.ID).Scan(&owner); err != nil {
			return false, wrapFalseCancellation(err)
		}
		// The backfill closed the meeting on this original alone, so only a
		// readable original can show that today's rules keep it on.
		settlement, readable, err := storedSettlementTx(ctx, tx, c, owner)
		return readable && settlement != meetingmap.SettleCancel, err
	default:
		return false, nil
	}
}

// storedSettlementTx reads the original stored under the cancelling event's
// key under today's rules, as owner's calendar sees it.
func storedSettlementTx(
	ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation, owner string,
) (meetingmap.Settlement, bool, error) {
	var payload []byte
	err := tx.QueryRow(ctx, `SELECT payload FROM raw_capture WHERE source_system = $1 AND source_id = $2`,
		c.Key.SourceSystem, c.Key.SourceID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, wrapFalseCancellation(err)
	}
	settlement, readable := storedMeetingSettlement(c.Key.SourceSystem, owner, payload)
	return settlement, readable, nil
}

func wrapFalseCancellation(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("compose: reading who cancelled a calendar meeting: %w", err)
}
