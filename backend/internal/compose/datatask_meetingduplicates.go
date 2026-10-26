// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingDuplicatesDataTask retires a Google Calendar copy of a meeting another
// door already held, such as a HubSpot import. It archives the copy when a sync
// today would bind the event to the holder. Nothing is merged: the survivor
// keeps its outcome, links and guests, and gains the copy's seat as importer.
// A stored Graph event names no calendar of its own, so it is not read.
func meetingDuplicatesDataTask(pool *pgxpool.Pool) datatask.Task {
	meetings := activities.NewStore(InstallationDB(pool))
	return datatask.Task{
		ID: "core/1793054210_calendar_meeting_duplicates", Version: 1,
		Title: "Retiring calendar copies of meetings stored twice",
		Steps: []datatask.Step{{
			Name: "retire_calendar_duplicates", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMeetingDuplicates,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return retireMeetingDuplicatesBatch(ctx, tx, meetings, in)
			},
		}},
	}
}

// calendarCopiesSQL selects the stored Google Calendar originals whose own
// meeting is live and holds no meeting identity. Another row held it when the
// event arrived. A meeting with a booking behind it is left alone.
const calendarCopiesSQL = `
	FROM raw_capture rc
	JOIN activity own ON own.source_system = rc.source_system AND own.source_id = rc.source_id
	WHERE rc.source_system = $1
	  AND own.kind = 'meeting' AND own.archived_at IS NULL AND own.restricted_at IS NULL
	  AND NOT EXISTS (SELECT 1 FROM activity_identity held
	                   WHERE held.activity_id = own.id AND held.identity_kind = 'meeting')
	  AND NOT EXISTS (SELECT 1 FROM meeting_invitation booked WHERE booked.activity_id = own.id)`

func retireMeetingDuplicatesBatch(ctx context.Context, tx pgx.Tx, meetings *activities.Store, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT rc.id, own.id, `+auth.ConnectorSeatSQL("own")+`::uuid, rc.payload `+calendarCopiesSQL+`
		  AND rc.id > $2 ORDER BY rc.id LIMIT $3`, sourceGCal, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: listing calendar copies of meetings: %w", err)
	}
	type calendarCopy struct {
		original ids.UUID
		meeting  ids.UUID
		seat     *ids.UUID
		payload  []byte
	}
	copies, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (calendarCopy, error) {
		var c calendarCopy
		return c, row.Scan(&c.original, &c.meeting, &c.seat, &c.payload)
	})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: reading calendar copies of meetings: %w", err)
	}
	scanned := make([]ids.UUID, 0, len(copies))
	survivors := []ids.ActivityID{}
	for _, c := range copies {
		scanned = append(scanned, c.original)
		if c.seat == nil {
			continue
		}
		survivor, found, err := meetingTheCopyBindsTo(ctx, tx, *c.seat, c.payload)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		duplicate := ids.From[ids.ActivityKind](c.meeting)
		if !found || survivor == duplicate {
			continue
		}
		if err := capture.CarryMeetingImportsTx(ctx, tx, duplicate, survivor); err != nil {
			return datatask.BatchOut{}, err
		}
		if err := meetings.ArchiveActivityTx(ctx, tx, duplicate, nil); err != nil {
			return datatask.BatchOut{}, err
		}
		survivors = append(survivors, survivor)
	}
	// The survivors' audiences follow the seats now recorded on them.
	if err := activities.RecomputeAudiencesTx(ctx, tx, survivors); err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, len(survivors))
}

// meetingTheCopyBindsTo answers the meeting a calendar sync for seat would
// bind this stored event to today, asked of the rule live capture asks. An
// original no parser reads, or one naming no calendar, binds to nothing.
func meetingTheCopyBindsTo(ctx context.Context, tx pgx.Tx, seat ids.UUID, payload []byte) (ids.ActivityID, bool, error) {
	raw, err := decodeStoredOriginal(payload)
	if err != nil {
		return ids.ActivityID{}, false, nil //nolint:nilerr // an original no parser reads is skipped, as live capture skips it
	}
	calendar, err := gcal.SelfAddress(raw)
	if err != nil || calendar == "" {
		return ids.ActivityID{}, false, nil //nolint:nilerr // an unreadable original, or one naming no calendar, proves no seat
	}
	rec, err := gcal.RecordOf(raw, calendar)
	if err != nil || !rec.CrossDoorIdentity.Stated() {
		return ids.ActivityID{}, false, nil //nolint:nilerr // a drop live capture would make is a drop here too
	}
	key := activities.MeetingIdentityKey(rec.CrossDoorIdentity.Series, capture.OccurrenceText(rec.CrossDoorIdentity))
	// The arriving seat is who the binding rule asks about, as a sync for that
	// seat's calendar binds.
	arriving := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:" + sourceGCal + ":" + seat.String(), UserID: seat,
	})
	resolve := activities.ResolveBindableIdentityProving(capture.ProvedUnambiguouslyTx)
	return resolve(arriving, tx, activities.IdentityKindMeeting, key)
}

func estimateMeetingDuplicates(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+calendarCopiesSQL+` LIMIT $2) capped`,
		sourceGCal, maximum+1).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("compose: estimating calendar copies of meetings: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}
