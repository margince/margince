// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// meetingPairsDataTask folds a Google Calendar meeting onto the meeting its
// seat's calendar binds the event to today, when the two are different live
// rows. Unlike meetingDuplicatesDataTask it also reads rows that hold an
// identity, which two seats' syncs leave when both stored one meeting. A
// stored original with an old start binds to itself and is left to the seat's
// next sync. Each fold is undone by unarchiving the retired row.
func meetingPairsDataTask(pool *pgxpool.Pool) datatask.Task {
	meetings := activities.NewStore(InstallationDB(pool))
	return datatask.Task{
		ID: "core/1793634207_calendar_meeting_pairs", Version: 1,
		Title: "Folding meetings two calendars stored twice",
		Steps: []datatask.Step{{
			Name: "fold_calendar_meeting_pairs", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateCalendarCopies(calendarOriginalsSQL),
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return retireCalendarCopiesBatch(ctx, tx, meetings, in, calendarOriginalsSQL)
			},
		}},
	}
}
