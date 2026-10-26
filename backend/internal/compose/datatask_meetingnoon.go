// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingNoonStartsDataTask puts back the start of a meeting the old outcome
// dialog moved to noon. Each restore is audited per record, and a restored
// meeting leaves the selection. Version 2 also restores a meeting archived
// after the damage.
//
// Two cases stay unrepaired, as #8671 accepted. A save after the cutoff has no
// evidence that the old dialog wrote it. A start read in a zone changed since
// may no longer read as noon. Neither start is guessed.
func meetingNoonStartsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792912000_meeting_noon_starts", Version: 2,
		Title: "Restore meeting times an old outcome save moved to noon",
		// The selection also reads occurred_precision, which core/1792983858 adds.
		Requires: []string{"core/" + activities.MeetingNoonCutoffMigration, "core/1792983858"},
		Steps: []datatask.Step{{
			Name: "restore_noon_starts", Limit: 100, MinLimit: 10,
			Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				tx, ok := q.(pgx.Tx)
				if !ok {
					return 0, false, errors.New("meeting noon estimate: the executor passed no transaction")
				}
				zone, err := identity.TimezoneAppliedTx(ctx, tx)
				if err != nil {
					return 0, false, err
				}
				count, err := activities.CountNoonMeetingsTx(ctx, tx, zone, maximum+1)
				return min(count, maximum), count > maximum, err
			},
			Batch: restoreNoonMeetingsBatch,
		}},
	}
}

func restoreNoonMeetingsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	zone, err := identity.TimezoneAppliedTx(ctx, tx)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	found, err := activities.NoonMeetingsAfterTx(ctx, tx, zone, after, min(in.Limit, math.MaxInt32))
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(found))
	changed := 0
	for _, meeting := range found {
		scanned = append(scanned, meeting.ID)
		restored, err := activities.RestoreMeetingStartTx(ctx, tx, meeting)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if restored {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}
