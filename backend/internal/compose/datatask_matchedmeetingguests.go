// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/meetingrosters"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// matchedMeetingGuestsDataTask gives a meeting that a Google Calendar event
// resolved onto, an import for instance, the guests the stored event lists.
// meetingrosters.GraphGuestsDataTask does the same for Microsoft events.
func matchedMeetingGuestsDataTask(pool *pgxpool.Pool) datatask.Task {
	inWorkspace := installationContext(pool)
	authority := IdentityService(pool)
	return datatask.Task{
		ID: "core/1792998483_matched_meeting_guests", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1792998483_matched_meeting_guests"), Requires: []string{"core/1792998482"},
		Steps: []datatask.Step{{
			Name: "add_calendar_guests", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMatchedMeetingOriginals,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				ctx, err := inWorkspace(ctx)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return addMatchedMeetingGuestsBatch(ctx, tx, authority, in)
			},
		}},
	}
}

// installationContext binds the installation's workspace on a data task's
// system context. A seat's authority is resolved per workspace.
func installationContext(pool *pgxpool.Pool) func(context.Context) (context.Context, error) {
	db := InstallationDB(pool)
	return func(ctx context.Context) (context.Context, error) {
		if _, bound := principal.WorkspaceID(ctx); bound {
			return ctx, nil
		}
		ws, err := db.Workspace(ctx)
		if err != nil {
			return ctx, err
		}
		return principal.WithWorkspaceID(ctx, ws.UUID), nil
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
	rows, err := tx.Query(ctx, `SELECT rc.id, `+originalstore.SQL("rc")+` `+matchedOriginalsSQL+`
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
		o.payload, err = originalstore.RepairPayload(ctx, o.payload)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		added, err := meetingrosters.AddGuestsFromOriginal(ctx, tx, authority, sourceGCal, o.payload, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if added {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
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
