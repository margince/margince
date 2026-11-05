// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// matchedMeetingGuestsDataTask gives a meeting that a Google Calendar event
// resolved onto, an import for instance, the guests the stored event lists.
// A stored Graph event names no calendar of its own, so it is not read.
func matchedMeetingGuestsDataTask(pool *pgxpool.Pool) datatask.Task {
	db := InstallationDB(pool)
	authority := identity.NewService(pool)
	return datatask.Task{
		ID: "core/1792998483_matched_meeting_guests", Version: 1,
		Title: "Adding calendar guests to matched meetings", Requires: []string{"core/1792998482"},
		Steps: []datatask.Step{{
			Name: "add_calendar_guests", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMatchedMeetingOriginals,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				// The stamping seat's authority is resolved per workspace,
				// which the executor's system context does not carry.
				if _, bound := principal.WorkspaceID(ctx); !bound {
					ws, err := db.Workspace(ctx)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					ctx = principal.WithWorkspaceID(ctx, ws.UUID)
				}
				return addMatchedMeetingGuestsBatch(ctx, tx, authority, in)
			},
		}},
	}
}

// matchedOriginalsSQL selects the stored Google Calendar originals whose own key
// holds no meeting, because the event was filed under a row another door holds.
const matchedOriginalsSQL = `
	FROM raw_capture rc
	WHERE rc.source_system = $1
	  AND NOT EXISTS (SELECT 1 FROM activity own
	                   WHERE own.source_system = rc.source_system AND own.source_id = rc.source_id
	                     AND own.restricted_at IS NULL)`

func addMatchedMeetingGuestsBatch(ctx context.Context, tx pgx.Tx, authority authz.Resolver, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT rc.id, rc.payload `+matchedOriginalsSQL+`
		  AND rc.id > $2 ORDER BY rc.id LIMIT $3`, sourceGCal, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: listing stored calendar originals: %w", err)
	}
	type original struct {
		id      ids.UUID
		payload []byte
	}
	originals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (original, error) {
		var o original
		return o, row.Scan(&o.id, &o.payload)
	})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: reading stored calendar originals: %w", err)
	}
	scanned := make([]ids.UUID, 0, len(originals))
	changed := 0
	for _, o := range originals {
		scanned = append(scanned, o.id)
		added, err := addGuestsFromOriginal(ctx, tx, authority, o.payload, in.RunID)
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
// identity names, when the calendar that stored it is the seat that stamps it.
func addGuestsFromOriginal(ctx context.Context, tx pgx.Tx, authority authz.Resolver, payload []byte, run ids.UUID) (bool, error) {
	raw, err := decodeStoredOriginal(payload)
	if err != nil {
		return false, nil //nolint:nilerr // an original no parser reads is skipped, as live capture skips it
	}
	calendar, err := gcal.SelfAddress(raw)
	if err != nil || calendar == "" {
		return false, nil //nolint:nilerr // an unreadable original, or one naming no calendar, proves no seat
	}
	rec, err := gcal.RecordOf(raw, calendar)
	if err != nil || !rec.CrossDoorIdentity.Stated() {
		return false, nil //nolint:nilerr // a drop live capture would make is a drop here too
	}
	key := activities.MeetingIdentityKey(rec.CrossDoorIdentity.Series, capture.OccurrenceText(rec.CrossDoorIdentity))
	meeting, found, err := activities.ResolveIdentity(ctx, tx, activities.IdentityKindMeeting, key)
	if err != nil || !found {
		return false, err
	}
	seat, stamps, err := stampingCalendarSeat(ctx, tx, meeting, calendar)
	if err != nil || !stamps {
		return false, err
	}
	added, err := capture.SettleStoredMeetingRosterTx(ctx, tx, authority, sourceGCal, meeting, seat, rec,
		map[string]any{machineAddressTaggedBy: run.String()})
	if err != nil || added.Added == 0 {
		return added.Changed(), err
	}
	return true, contacts.FillParticipantNamesTx(ctx, tx, meeting)
}

// stampingCalendarSeat answers the seat whose Google Calendar stamps the live
// meeting, and only when that calendar is the one the stored event came from.
func stampingCalendarSeat(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, calendar string) (ids.UUID, bool, error) {
	var seat *ids.UUID
	var label string
	err := tx.QueryRow(ctx, `
		SELECT `+auth.ConnectorSeatSQL("a")+`::uuid, coalesce(c.account_label, '')
		  FROM activity a
		  LEFT JOIN capture_connection c
		    ON c.provider = $2 AND c.user_id::text = `+auth.ConnectorSeatSQL("a")+`
		 WHERE a.id = $1 AND a.kind = 'meeting' AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND split_part(a.captured_by, ':', 2) = $2
		 LIMIT 1`, meeting, sourceGCal).Scan(&seat, &label)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("compose: reading the calendar that stamps a matched meeting: %w", err)
	}
	if seat == nil || !strings.EqualFold(strings.TrimSpace(label), calendar) {
		return ids.Nil, false, nil
	}
	return *seat, true, nil
}

func estimateMatchedMeetingOriginals(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+matchedOriginalsSQL+` LIMIT $2) capped`,
		sourceGCal, maximum+1).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("compose: estimating stored calendar originals: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}
