// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// meetingHistoryPrecisionDataTask records the precision of the meeting
// history starts the noon repair restored before the column existed. A run of
// that repair after this column writes the precision itself, so the two can
// run in either order. Any other old start stays unknown.
func meetingHistoryPrecisionDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793336397_meeting_history_precision", Version: 1,
		Title:    "Recording which restored meeting times are exact",
		Requires: []string{"core/1793336397"},
		Steps: []datatask.Step{{
			Name: "prove_restored_instants", Limit: 500, MinLimit: 10,
			Audit:    datatask.AuditPolicy{Reason: "activities.recordMeetingTransition stamps the same precision on every new transition, without a separate audit row"},
			Estimate: estimateUnknownMeetingPrecision, Batch: proveRestoredInstantsBatch,
		}},
	}
}

func proveRestoredInstantsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	repair := datatask.ActorID(meetingNoonStartsDataTask().ID)
	page, err := activities.ProveRestoredInstantsTx(ctx, tx, repair, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Proved)
}

func estimateUnknownMeetingPrecision(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	count, err := activities.EstimateUnknownMeetingPrecision(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
