// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Older calendar code cancelled a shared meeting when one attendee declined or
// removed their own copy. The evidence for reopening is who cancelled it and
// the stored original read under today's rules. A status a human or an agent
// set stands, and so does anything the evidence does not explain.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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

// falseCancelCursor walks the scope by id, then once more from the start. A
// meeting cancelled behind the first walk, by a sync still running older code,
// is reached by the second.
type falseCancelCursor struct {
	After   ids.UUID `json:"after"`
	Recheck bool     `json:"recheck"`
}

func readFalseCancelCursor(raw json.RawMessage) (falseCancelCursor, error) {
	var cursor falseCancelCursor
	if len(raw) == 0 {
		return cursor, nil
	}
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return falseCancelCursor{}, &storekit.MalformedCursorError{}
	}
	return cursor, nil
}

func reopenFalseCancellationsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, since time.Time) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	cursor, err := readFalseCancelCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	var out datatask.BatchOut
	for {
		found, err := activities.CalendarCancellationsAfterTx(ctx, tx, since, cursor.After, in.Limit)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		reopened, err := reopenGuestCancellationsTx(ctx, tx, found, &cursor, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		out.Scanned, out.Changed = len(found), reopened
		if len(found) == in.Limit {
			break
		}
		if cursor.Recheck {
			out.Done = true
			break
		}
		cursor = falseCancelCursor{Recheck: true}
		// An empty page reports no progress, so the second walk starts here.
		if len(found) > 0 {
			break
		}
	}
	if out.Cursor, err = json.Marshal(cursor); err != nil {
		return datatask.BatchOut{}, err
	}
	return out, nil
}

// reopenGuestCancellationsTx reopens each found meeting a guest's calendar
// cancelled for everybody, moving cursor past every meeting it read.
func reopenGuestCancellationsTx(ctx context.Context, tx pgx.Tx, found []activities.CalendarCancellation, cursor *falseCancelCursor, runID ids.UUID) (int, error) {
	reopened := 0
	for _, c := range found {
		cursor.After = c.ID
		guest, err := cancelledForAGuestTx(ctx, tx, c)
		if err != nil {
			return 0, err
		}
		if !guest {
			continue
		}
		done, err := activities.ReopenCalendarCancellationTx(ctx, tx, c, runID)
		if err != nil {
			return 0, err
		}
		if done {
			reopened++
		}
	}
	return reopened, nil
}

// cancelledForAGuestTx answers whether the evidence shows a guest's calendar
// cancelled the meeting rather than its organizer.
func cancelledForAGuestTx(ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation) (bool, error) {
	newest, err := cancellationIsTheLastSignTx(ctx, tx, c)
	if err != nil || !newest {
		return false, err
	}
	switch {
	case c.ActorType == string(principal.PrincipalConnector) && c.Seat != nil:
		return declinedBySeatTx(ctx, tx, c, *c.Seat)
	case c.ActorType == string(principal.PrincipalSystem):
		return closedOnAnOriginalNowOnTx(ctx, tx, c)
	default:
		return false, nil
	}
}

// declinedBySeatTx answers whether a calendar cancelled the meeting for seat
// because seat declined it, and not because its organizer called it off.
func declinedBySeatTx(ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation, seat ids.UUID) (bool, error) {
	organizer, err := capture.SeatOrganizesMeetingTx(ctx, tx, ids.From[ids.ActivityKind](c.ID), seat)
	if err != nil || organizer {
		return false, err
	}
	var owner string
	if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT account_label FROM capture_connection
 WHERE provider = $1 AND user_id = $2 LIMIT 1), '')`, c.Key.SourceSystem, seat).Scan(&owner); err != nil {
		return false, wrapFalseCancellation(err)
	}
	payload, stored, err := cancellingOriginalTx(ctx, tx, c)
	if err != nil {
		return false, err
	}
	if settlement, readable := storedMeetingSettlement(c.Key.SourceSystem, owner, payload); stored && readable && settlement == meetingmap.SettleCancel {
		return false, nil
	}
	// A guest's calendar also reports an organizer's cancellation, so only
	// the seat's own decline shows the cancellation was not the organizer's.
	return seatDeclinedTx(ctx, tx, c, seat, owner, payload)
}

// closedOnAnOriginalNowOnTx answers whether the stored-original backfill closed
// the meeting on an original that today's rules read as still on.
func closedOnAnOriginalNowOnTx(ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation) (bool, error) {
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
	payload, stored, err := cancellingOriginalTx(ctx, tx, c)
	if err != nil || !stored {
		return false, err
	}
	settlement, readable := storedMeetingSettlement(c.Key.SourceSystem, owner, payload)
	return readable && settlement != meetingmap.SettleCancel, nil
}

// cancellingOriginalTx reads the original stored under the cancelling event's
// key, and whether one is stored.
func cancellingOriginalTx(ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation) ([]byte, bool, error) {
	var payload []byte
	err := tx.QueryRow(ctx, `SELECT `+originalstore.SQL("r")+` FROM raw_capture r WHERE source_system = $1 AND source_id = $2`,
		c.Key.SourceSystem, c.Key.SourceID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, wrapFalseCancellation(err)
	}
	payload, err = originalstore.RepairPayload(ctx, payload)
	return payload, err == nil, err
}

// seatDeclinedTx answers whether the cancelling seat declined the meeting: its
// own row holds a declined answer, or its stored original does.
func seatDeclinedTx(
	ctx context.Context, tx pgx.Tx, c activities.CalendarCancellation, seat ids.UUID, owner string, payload []byte,
) (bool, error) {
	var declined bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity_participant p JOIN activity a ON a.id = p.activity_id
 WHERE p.activity_id = $1 AND p.user_id = $2 AND p.response = $3
 AND a.archived_at IS NULL AND a.restricted_at IS NULL AND `+auth.ParticipantStandsSQL("p", "a")+`)`, c.ID, seat, connector.ResponseDeclined).Scan(&declined); err != nil || declined {
		return declined, wrapFalseCancellation(err)
	}
	return storedAnswer(c.Key.SourceSystem, owner, payload) == connector.ResponseDeclined, nil
}

// storedAnswer is the calendar owner's own answer in a stored original, or ""
// when the original states none or cannot be read.
func storedAnswer(source, owner string, payload []byte) string {
	raw, err := decodeStoredOriginal(payload)
	if err != nil || owner == "" {
		return ""
	}
	read := gcal.RecordOf
	if source == sourceGraphCal {
		read = graphcal.RecordOf
	}
	rec, err := read(raw, owner)
	if err != nil || rec.CalendarAnswer == nil {
		return ""
	}
	return rec.CalendarAnswer.Response
}

func wrapFalseCancellation(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("compose: reading who cancelled a calendar meeting: %w", err)
}
