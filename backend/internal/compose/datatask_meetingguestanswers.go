// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// meetingGuestAnswersDataTask fills each guest's answer, and a crowded
// meeting's outside guests, from the stored Google Calendar event, as the seat
// whose calendar stamps the meeting. A stored answer only fills an empty one.
func meetingGuestAnswersDataTask(pool *pgxpool.Pool) datatask.Task {
	db := InstallationDB(pool)
	authority := identity.NewService(pool)
	return datatask.Task{
		ID: "core/1793003497_meeting_guest_answers", Version: 1,
		Title: "Recording what calendar guests answered", Requires: []string{"core/1793003496"},
		Steps: []datatask.Step{{
			Name: "fill_guest_answers", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateStoredCalendarOriginals,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				if _, bound := principal.WorkspaceID(ctx); !bound {
					ws, err := db.Workspace(ctx)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					ctx = principal.WithWorkspaceID(ctx, ws.UUID)
				}
				return fillGuestAnswersBatch(ctx, tx, authority, in)
			},
		}},
	}
}

func fillGuestAnswersBatch(ctx context.Context, tx pgx.Tx, authority authz.Resolver, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT rc.id, rc.source_id, rc.payload FROM raw_capture rc
		 WHERE rc.source_system = $1 AND rc.id > $2 ORDER BY rc.id LIMIT $3`, sourceGCal, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: listing stored calendar originals: %w", err)
	}
	type original struct {
		id       ids.UUID
		sourceID string
		payload  []byte
	}
	originals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (original, error) {
		var o original
		return o, row.Scan(&o.id, &o.sourceID, &o.payload)
	})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: reading stored calendar originals: %w", err)
	}
	scanned := make([]ids.UUID, 0, len(originals))
	changed := 0
	for _, o := range originals {
		scanned = append(scanned, o.id)
		settled, err := fillAnswersFromOriginal(ctx, tx, authority, o.sourceID, o.payload, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if settled {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// fillAnswersFromOriginal settles one stored event on the meeting its own key
// or its identity names, when the calendar that stored it stamps that meeting.
func fillAnswersFromOriginal(
	ctx context.Context, tx pgx.Tx, authority authz.Resolver, sourceID string, payload []byte, run ids.UUID,
) (bool, error) {
	raw, err := decodeStoredOriginal(payload)
	if err != nil {
		return false, nil //nolint:nilerr // an original no parser reads is skipped, as live capture skips it
	}
	calendar, err := gcal.SelfAddress(raw)
	if err != nil || calendar == "" {
		return false, nil //nolint:nilerr // an unreadable original, or one naming no calendar, proves no seat
	}
	rec, err := gcal.RecordOf(raw, calendar)
	if err != nil {
		return false, nil //nolint:nilerr // a drop live capture would make is a drop here too
	}
	meeting, found, err := storedEventMeeting(ctx, tx, sourceID, rec.CrossDoorIdentity)
	if err != nil || !found {
		return false, err
	}
	seat, stamps, err := stampingCalendarSeat(ctx, tx, meeting, calendar)
	if err != nil || !stamps {
		return false, err
	}
	settled, err := capture.SettleStoredMeetingRosterTx(ctx, tx, authority, sourceGCal, meeting, seat, rec,
		map[string]any{machineAddressTaggedBy: run.String()})
	return settled.Changed(), err
}

// storedEventMeeting is the live meeting under the event's own key, else the
// one its calendar identity resolved onto.
func storedEventMeeting(
	ctx context.Context, tx pgx.Tx, sourceID string, identity connector.CrossDoorIdentity,
) (ids.ActivityID, bool, error) {
	var own ids.ActivityID
	err := tx.QueryRow(ctx, `
		SELECT id FROM activity WHERE source_system = $1 AND source_id = $2 AND restricted_at IS NULL`,
		sourceGCal, sourceID).Scan(&own)
	if err == nil {
		return own, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, fmt.Errorf("compose: reading the meeting a stored event made: %w", err)
	}
	if !identity.Stated() {
		return ids.ActivityID{}, false, nil
	}
	key := activities.MeetingIdentityKey(identity.Series, capture.OccurrenceText(identity))
	return activities.ResolveIdentity(ctx, tx, activities.IdentityKindMeeting, key)
}

func estimateStoredCalendarOriginals(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM raw_capture WHERE source_system = $1 LIMIT $2) capped`,
		sourceGCal, maximum+1).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("compose: estimating stored calendar originals: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}
