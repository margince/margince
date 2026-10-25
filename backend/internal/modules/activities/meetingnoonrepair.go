// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The meetings an old Worklist outcome dialog moved to noon, and the write that
// puts the start back. That dialog sent a noon start in the same save as the
// outcome. A later release stopped it. A save made before that release's first
// migration ran is taken as the old dialog's, and the audit row's start returns.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MeetingNoonCutoffMigration is the first core migration merged after the
// dialog stopped sending noon. When it ran marks the first start of code that
// no longer does, so an outcome save before it counts as the old dialog's.
const MeetingNoonCutoffMigration = "1792761600"

// noonMeetingScope selects meetings whose last start change is an update made
// before the cutoff. A human actor made it, and it set the outcome with a start at
// noon of the day the start already had. No later change moved the start. The
// zone is $1, the cutoff migration $2.
const noonMeetingScope = `
  FROM activity a
  JOIN LATERAL (
       SELECT au.action, au.actor_type, au.occurred_at AS saved_at, au.after ? 'meeting_status' AS set_status,
              (au.before ->> 'occurred_at')::timestamptz AS original,
              (au.after ->> 'occurred_at')::timestamptz AS moved
         FROM audit_log au
        WHERE au.entity_type = 'activity' AND au.entity_id = a.id AND au.after ? 'occurred_at'
        ORDER BY au.occurred_at DESC, au.id DESC LIMIT 1) last ON true
 WHERE a.kind = 'meeting' AND a.meeting_status IS NOT NULL
   AND a.restricted_at IS NULL AND a.archived_at IS NULL
   AND (a.occurred_at AT TIME ZONE $1)::time = TIME '12:00:00'
   AND last.action = 'update' AND last.actor_type = 'human' AND last.set_status AND last.moved = a.occurred_at AND last.original IS NOT NULL
   AND (last.original AT TIME ZONE $1)::date = (a.occurred_at AT TIME ZONE $1)::date
   AND (last.original AT TIME ZONE $1)::time <> TIME '12:00:00'
   AND last.saved_at < (SELECT m.applied_at FROM schema_migrations_core m WHERE m.version = $2)
   AND NOT EXISTS (SELECT 1 FROM meeting_invitation i WHERE i.activity_id = a.id AND i.status <> 'canceled')`

// NoonMeeting is one meeting to restore: the start it holds and the one the
// outcome save replaced.
type NoonMeeting struct {
	ID       ids.UUID
	Version  int64
	Noon     time.Time
	Original time.Time
}

// NoonMeetingsAfterTx reads the next limit meetings after the cursor in id
// order, in the installation zone. A restored meeting leaves the selection, so
// a second run finds none.
func NoonMeetingsAfterTx(ctx context.Context, tx pgx.Tx, zone string, after ids.UUID, limit int) ([]NoonMeeting, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.version, a.occurred_at, last.original `+noonMeetingScope+`
		   AND a.id > $3 ORDER BY a.id LIMIT $4`, zone, MeetingNoonCutoffMigration, after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: selecting meetings an outcome save moved to noon: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowToStructByPos[NoonMeeting])
	if err != nil {
		return nil, fmt.Errorf("activities: reading meetings an outcome save moved to noon: %w", err)
	}
	return found, nil
}

// CountNoonMeetingsTx counts the meetings still to restore, up to limit.
func CountNoonMeetingsTx(ctx context.Context, q pgx.Tx, zone string, limit int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id `+noonMeetingScope+` LIMIT $3) found`,
		zone, MeetingNoonCutoffMigration, limit).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("activities: counting meetings an outcome save moved to noon: %w", err)
	}
	return count, nil
}

// RestoreMeetingStartTx puts a meeting's start back through the module's own
// update, and on its history rows. That update locks the row, bumps the version,
// audits, emits the event and moves the last-activity clocks. It answers false
// and writes nothing when the row changed since it was read.
func RestoreMeetingStartTx(ctx context.Context, tx pgx.Tx, m NoonMeeting) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	_, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](m.ID), UpdateActivityInput{
		OccurredAt: &m.Original, IfVersion: &m.Version,
	})
	if errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: restoring a meeting start: %w", err)
	}
	// The outcome save also recorded the noon start on its history row, which
	// the held-meetings report prefers over the activity's own start.
	if _, err := tx.Exec(ctx, `UPDATE activity_meeting_history SET scheduled_start = $2
		WHERE activity_id = $1 AND scheduled_start = $3`, m.ID, m.Original, m.Noon); err != nil {
		return false, fmt.Errorf("activities: restoring a meeting's history start: %w", err)
	}
	return true, nil
}
