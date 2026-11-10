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
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingSeriesSlotsDataTask keys each stored occurrence of a recurring
// meeting by its slot in the series, as capture keys it now. A meeting keyed
// by its moved start before then could take another occurrence's key, and
// a sync folded that occurrence onto it. The second step splits such a fold
// where unarchiving the folded row gives back what the fold took, and logs
// every other one for review.
func meetingSeriesSlotsDataTask(pool *pgxpool.Pool) datatask.Task {
	meetings := activities.NewStore(InstallationDB(pool))
	return datatask.Task{
		ID: "core/1793916347_meeting_series_slot_identities", Version: 1,
		Title: "Keying recurring meetings by their slot in the series",
		Steps: []datatask.Step{{
			Name: "rekey_series_slots", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{Reason: "every calendar sync claims and retires the same meeting identities without an audit (activities.ClaimIdentity)"},
			Estimate: estimateStoredOriginals(liveSeriesOriginalsSQL, seriesCalendars),
			Batch:    rekeySeriesSlotsBatch,
		}, {
			Name: "split_folded_occurrences", Limit: 50, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateStoredOriginals(foldedSeriesOriginalsSQL, seriesCalendars),
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return splitFoldedOccurrencesBatch(ctx, tx, meetings, in)
			},
		}},
	}
}

// seriesCalendars are the calendars whose stored events name a series slot.
var seriesCalendars = []string{sourceGCal, sourceGraphCal}

// seriesOriginalsSQL selects the stored calendar originals of meetings.
const seriesOriginalsSQL = `
	FROM raw_capture rc
	JOIN activity own ON own.source_system = rc.source_system AND own.source_id = rc.source_id
	WHERE rc.source_system = ANY($1) AND own.kind = 'meeting'`

// The live step reads unrestricted meetings only. The split step lists
// restricted folds too, so it can log that it skips them.
const (
	liveSeriesOriginalsSQL   = seriesOriginalsSQL + ` AND own.archived_at IS NULL AND own.restricted_at IS NULL`
	foldedSeriesOriginalsSQL = seriesOriginalsSQL + ` AND own.archived_at IS NOT NULL`
)

// storedOccurrence is one stored calendar original and the meeting filed under
// its key. A restricted meeting's original is not read, so its payload is nil.
type storedOccurrence struct {
	original   ids.UUID
	meeting    ids.ActivityID
	source     string
	capturedBy string
	payload    []byte
}

func listStoredOccurrences(ctx context.Context, tx pgx.Tx, selection string, in datatask.BatchIn) ([]storedOccurrence, error) {
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT rc.id, own.id, rc.source_system, own.captured_by,
		       CASE WHEN own.restricted_at IS NULL THEN `+originalstore.SQL("rc")+` END `+selection+`
		  AND rc.id > $2 ORDER BY rc.id LIMIT $3`, seriesCalendars, after, in.Limit)
	if err != nil {
		return nil, fmt.Errorf("compose: listing stored meeting occurrences: %w", err)
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedOccurrence, error) {
		var o storedOccurrence
		return o, row.Scan(&o.original, &o.meeting, &o.source, &o.capturedBy, &o.payload)
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading stored meeting occurrences: %w", err)
	}
	return found, nil
}

// seriesSlot is the identity live capture files an occurrence under, and the
// series it belongs to.
type seriesSlot struct {
	series, key string
}

// slotOf reads the series slot a stored original names. An original no parser
// reads, or one that names no slot in a series, has none.
func slotOf(source string, payload []byte) (seriesSlot, bool) {
	raw, err := decodeStoredOriginal(payload)
	if err != nil {
		return seriesSlot{}, false
	}
	read := gcal.SeriesIdentityOf
	if source == sourceGraphCal {
		read = graphcal.SeriesIdentityOf
	}
	identity, inSeries, err := read(raw)
	if err != nil || !inSeries {
		return seriesSlot{}, false
	}
	return seriesSlot{
		series: identity.Series,
		key:    activities.MeetingIdentityKey(identity.Series, capture.OccurrenceText(identity)),
	}, true
}

// rekeySeriesSlotsBatch claims each live occurrence's slot key, which retires
// the key its start gave it before.
func rekeySeriesSlotsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	occurrences, err := listStoredOccurrences(ctx, tx, liveSeriesOriginalsSQL, in)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(occurrences))
	rekeyed := 0
	for _, o := range occurrences {
		scanned = append(scanned, o.original)
		o.payload, err = originalstore.RepairPayload(ctx, o.payload)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		slot, ok := slotOf(o.source, o.payload)
		if !ok {
			continue
		}
		changed, err := activities.RekeyMeetingIdentity(ctx, tx, o.meeting, slot.key, o.capturedBy)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if changed {
			rekeyed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, rekeyed)
}
