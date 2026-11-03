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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// organizerHostCandidatesSQL selects the synced meetings whose organizer is
// bound to a seat other than their host. ClaimOrganizerHostTx decides each one.
const organizerHostCandidatesSQL = `FROM activity a
	WHERE a.kind = 'meeting' AND a.archived_at IS NULL AND a.captured_by LIKE 'connector:%'
	  AND EXISTS (SELECT 1 FROM activity_participant op
	               WHERE op.activity_id = a.id AND op.role = 'organizer' AND op.user_id IS NOT NULL
	                 AND op.user_id IS DISTINCT FROM a.host_user_id)`

// meetingOrganizerHostDataTask hands each stored calendar meeting to its
// organizer, as a sync now does when it replays one. A meeting a guest's
// calendar synced first names that guest as host, and no later sync moves it.
func meetingOrganizerHostDataTask(pool *pgxpool.Pool) datatask.Task {
	meetings := activities.NewStore(InstallationDB(pool))
	return datatask.Task{
		ID: "core/1793690361_meeting_organizer_host", Version: 1,
		Title: "Naming each calendar meeting's organizer as its host",
		Steps: []datatask.Step{{
			Name: "claim_organizer_hosts", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateOrganizerHosts,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return claimOrganizerHostsBatch(ctx, tx, meetings, in)
			},
		}},
	}
}

// claimOrganizerHostsBatch claims the host of each candidate after the cursor.
func claimOrganizerHostsBatch(ctx context.Context, tx pgx.Tx, meetings *activities.Store, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id `+organizerHostCandidatesSQL+`
		  AND a.id > $1 ORDER BY a.id LIMIT $2`, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: listing meetings hosted by a guest: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("compose: reading meetings hosted by a guest: %w", err)
	}
	claimed := 0
	for _, id := range scanned {
		moved, err := meetings.ClaimOrganizerHostTx(ctx, tx, ids.From[ids.ActivityKind](id))
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if moved {
			claimed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, claimed)
}

// estimateOrganizerHosts counts the candidates, up to maximum.
func estimateOrganizerHosts(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+organizerHostCandidatesSQL+` LIMIT $1) capped`,
		maximum+1).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("compose: estimating meetings hosted by a guest: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}
