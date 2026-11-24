// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// splitSeriesDataTask folds the meetings a split series replaced, stored
// before capture folded them as they arrived. It asks each calendar meeting
// the rule capture asks of a new one. It folds through the fold capture uses,
// which leaves a row a human worked or one filed under more records.
func splitSeriesDataTask(pool *pgxpool.Pool) datatask.Task {
	meetings := activities.NewStore(InstallationDB(pool))
	return datatask.Task{
		ID: "core/1795540344_split_series_meetings", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1795540344_split_series_meetings"),
		Steps: []datatask.Step{{
			Name: "fold_replaced_series", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateStoredOriginals(seriesMeetingsSQL, seriesCalendars),
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return foldReplacedSeriesBatch(ctx, tx, meetings, in)
			},
		}},
	}
}

// seriesMeetingsSQL selects the live calendar meetings a seat's calendar
// stored, the rows a split series may have replaced another on.
var seriesMeetingsSQL = `
	FROM activity a
	WHERE a.source_system = ANY($1) AND a.kind = 'meeting'
	  AND a.archived_at IS NULL AND a.restricted_at IS NULL
	  AND ` + auth.ConnectorSeatSQL("a") + ` IS NOT NULL`

// foldReplacedSeriesBatch folds, for each meeting after the cursor, the older
// rows its series replaced at its slot onto it.
func foldReplacedSeriesBatch(
	ctx context.Context, tx pgx.Tx, meetings *activities.Store, in datatask.BatchIn,
) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, `+auth.ConnectorSeatSQL("a")+`::uuid `+seriesMeetingsSQL+`
		  AND a.id > $2 ORDER BY a.id LIMIT $3`, seriesCalendars, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: listing calendar meetings: %w", err)
	}
	type seatMeeting struct{ meeting, seat ids.UUID }
	page, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (seatMeeting, error) {
		var m seatMeeting
		return m, row.Scan(&m.meeting, &m.seat)
	})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: reading calendar meetings: %w", err)
	}
	scanned := make([]ids.UUID, 0, len(page))
	folded := 0
	for _, m := range page {
		scanned = append(scanned, m.meeting)
		survivor := ids.From[ids.ActivityKind](m.meeting)
		replaced, err := activities.ReplacedSeriesMeetingsTx(ctx, tx, survivor, m.seat)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		for _, old := range replaced {
			done, err := foldMeetingDuplicateTx(ctx, tx, meetings, old, survivor, m.seat, "")
			if err != nil {
				return datatask.BatchOut{}, err
			}
			if done {
				folded++
			}
		}
	}
	return datatask.AdvanceUUID(in, scanned, folded)
}
