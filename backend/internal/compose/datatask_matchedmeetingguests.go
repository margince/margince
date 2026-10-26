// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// matchedMeetingGuestsDataTask gives a meeting that a calendar event resolved
// onto, an import for instance, the guests that event's stored original lists.
func matchedMeetingGuestsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792990000_matched_meeting_guests", Version: 1,
		Title: "Adding calendar guests to matched meetings", Requires: []string{"core/1792969126"},
		Steps: []datatask.Step{{
			Name: "add_calendar_guests", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMatchedMeetingOriginals, Batch: addMatchedMeetingGuestsBatch,
		}},
	}
}

// matchedOriginalsSQL selects the stored calendar originals whose own key holds
// no meeting, because the event was filed under a row another door holds.
const matchedOriginalsSQL = `
	FROM raw_capture rc
	WHERE rc.source_system = ANY($1)
	  AND NOT EXISTS (SELECT 1 FROM activity own
	                   WHERE own.source_system = rc.source_system AND own.source_id = rc.source_id
	                     AND own.restricted_at IS NULL)`

func addMatchedMeetingGuestsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT rc.id, rc.source_system, rc.payload `+matchedOriginalsSQL+`
		  AND rc.id > $2 ORDER BY rc.id LIMIT $3`, []string{sourceGCal, sourceGraphCal}, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: listing stored calendar originals: %w", err)
	}
	type original struct {
		id      ids.UUID
		source  string
		payload []byte
	}
	originals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (original, error) {
		var o original
		return o, row.Scan(&o.id, &o.source, &o.payload)
	})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: reading stored calendar originals: %w", err)
	}
	scanned := make([]ids.UUID, 0, len(originals))
	changed := 0
	for _, o := range originals {
		scanned = append(scanned, o.id)
		added, err := addGuestsFromOriginal(ctx, tx, o.source, o.payload, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if added {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// addGuestsFromOriginal files one stored event's guests on the meeting its
// identity names. Only a meeting this calendar door now stamps is touched.
func addGuestsFromOriginal(ctx context.Context, tx pgx.Tx, source string, payload []byte, run ids.UUID) (bool, error) {
	rec, ok := storedCalendarRecord(source, payload)
	if !ok || !rec.CrossDoorIdentity.Stated() {
		return false, nil
	}
	key := activities.MeetingIdentityKey(rec.CrossDoorIdentity.Series, capture.OccurrenceText(rec.CrossDoorIdentity))
	meeting, found, err := activities.ResolveIdentity(ctx, tx, activities.IdentityKindMeeting, key)
	if err != nil || !found {
		return false, err
	}
	var seat *ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT `+auth.ConnectorSeatSQL("a")+`::uuid FROM activity a
		 WHERE a.id = $1 AND a.kind = 'meeting' AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND split_part(a.captured_by, ':', 2) = $2`, meeting, source).Scan(&seat); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("compose: reading the meeting a stored event matched: %w", err)
	}
	if seat == nil {
		return false, nil
	}
	parties := connector.Parties{Participants: rec.Participants, Withheld: rec.WithheldParties}
	added, err := capture.RecordLaterMeetingGuestsTx(ctx, tx, meeting, *seat, parties, true,
		map[string]any{"data_task_run": run.String()})
	if err != nil || !added.Changed() {
		return false, err
	}
	return true, contacts.FillParticipantNamesTx(ctx, tx, meeting)
}

// storedCalendarRecord decodes a stored original the way its connector would.
// An original no parser reads is skipped, as live capture skips it.
func storedCalendarRecord(source string, payload []byte) (connector.NormalizedRecord, bool) {
	raw, err := decodeStoredOriginal(payload)
	if err != nil {
		return connector.NormalizedRecord{}, false
	}
	var rec connector.NormalizedRecord
	switch source {
	case sourceGCal:
		rec, err = gcal.RecordOf(raw, "")
	case sourceGraphCal:
		rec, err = graphcal.RecordOf(raw, "")
	default:
		return connector.NormalizedRecord{}, false
	}
	return rec, err == nil
}

func estimateMatchedMeetingOriginals(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+matchedOriginalsSQL+` LIMIT $2) capped`,
		[]string{sourceGCal, sourceGraphCal}, maximum+1).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("compose: estimating stored calendar originals: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}
