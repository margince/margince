// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// calendarDescriptionsDataTask renders calendar meeting descriptions stored as
// raw HTML before capture rendered them. Each rewrite is the activity update,
// so it is audited per record. A rendered meeting carries the audit flag and
// leaves the selection, so a replay changes nothing.
func calendarDescriptionsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793007922_calendar_html_descriptions", Version: 1,
		Title: "Render calendar descriptions stored as HTML",
		Steps: []datatask.Step{{
			Name: "render_calendar_descriptions", Limit: 100, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateCalendarHTMLDescriptions, Batch: renderCalendarDescriptionsBatch,
		}},
	}
}

// calendarHTMLDescriptionScope is a live calendar meeting whose body holds a
// "<", not yet rendered by this task. Whether the "<" opens an HTML tag is
// decided per row, by the capture code (meetingmap.RerenderStoredBody).
const calendarHTMLDescriptionScope = `FROM activity a
 WHERE a.kind = 'meeting' AND a.restricted_at IS NULL AND a.archived_at IS NULL
   AND a.body IS NOT NULL AND strpos(a.body, '<') > 0
   AND split_part(a.captured_by, ':', 1) = 'connector' AND split_part(a.captured_by, ':', 2) IN ('gcal', 'graphcal')
   AND NOT EXISTS (SELECT 1 FROM audit_log rendered
         WHERE rendered.entity_type = 'activity' AND rendered.entity_id = a.id
           AND rendered.after ? '` + activities.MeetingDescriptionRenderedFlag + `')`

type storedMeetingBody struct {
	ID      ids.UUID
	Version int64
	Body    string
}

func renderCalendarDescriptionsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.version, a.body `+calendarHTMLDescriptionScope+`
		AND a.id > @described_after ORDER BY a.id LIMIT @described_limit FOR UPDATE OF a`,
		pgx.NamedArgs{"described_after": after, "described_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[storedMeetingBody])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	changed := 0
	for _, candidate := range candidates {
		scanned = append(scanned, candidate.ID)
		body, differs := meetingmap.RerenderStoredBody(candidate.Body)
		if !differs {
			continue
		}
		rendered, err := activities.RenderMeetingBodyTx(ctx, tx, activities.MeetingBodyRender{
			Activity: candidate.ID, Version: candidate.Version, StoredBody: candidate.Body, Body: body,
		})
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if rendered {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

func estimateCalendarHTMLDescriptions(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT a.id "+calendarHTMLDescriptionScope+" LIMIT @described_cap) candidates",
		pgx.NamedArgs{"described_cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
