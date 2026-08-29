// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aiactivity

// The AI-work-health read: the caller's own runs that went wrong — failed
// inside a bounded window, or live past the lease their source declared. It
// reads the same projection Mine reads and claims only AI work: email
// delivery, capture backfills and scheduled sends never reach ai_task_run,
// and a lane fed from here must not present itself as general job health.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TroubledRun is one of the caller's runs that went wrong: State is "failed"
// or the read-time StateStalled, and OccurredAt is when it failed or, for a
// stalled run, when it started.
type TroubledRun struct {
	ID           ids.UUID
	Kind         string
	State        string
	OccurredAt   time.Time
	Summary      *string
	SubjectLabel *string
}

// troubledSQL reads both failure shapes in ONE statement, for the same
// snapshot reason feedSQL does. The stalled arm decides staleness in SQL
// against the database clock, exactly as the live arm of feedSQL does; the
// failed arm bounds on finished_at because "what failed recently" is a
// question about when it failed.
const troubledSQL = `
(
  SELECT id, kind, '` + StateStalled + `',
         COALESCE(started_at, queued_at), left(summary, $4), left(subject_label, $5)
    FROM ai_task_run
   WHERE actor_user_id = $1
     AND state IN ('queued','running')
     AND stale_after IS NOT NULL AND stale_after < now()
   ORDER BY queued_at DESC, id DESC
   LIMIT $3
)
UNION ALL
(
  SELECT id, kind, state,
         COALESCE(finished_at, started_at, queued_at), left(summary, $4), left(subject_label, $5)
    FROM ai_task_run
   WHERE actor_user_id = $1
     AND state = 'failed'
     AND finished_at >= $2
   ORDER BY finished_at DESC, id DESC
   LIMIT $3
)`

// Troubled answers the calling person's own failed and stalled runs — failed
// since `since`, stalled right now — bounded per arm by limit, stalled first.
// The person comes from the bound principal and is not a parameter, for the
// reason Mine's doc states; a caller with no person behind it is refused with
// the permission sentinel, which the attention feed renders as a withheld
// lane rather than a broken day.
func (s *Store) Troubled(ctx context.Context, since time.Time, limit int) ([]TroubledRun, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, fmt.Errorf("aiactivity: a personal read needs an authenticated person: %w", apperrors.ErrPermissionDenied)
	}
	var troubled []TroubledRun
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, troubledSQL,
			actor.UserID, since, limit, SummaryBound, SubjectLabelBound)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		troubled = []TroubledRun{}
		for rows.Next() {
			var run TroubledRun
			if scanErr := rows.Scan(&run.ID, &run.Kind, &run.State,
				&run.OccurredAt, &run.Summary, &run.SubjectLabel); scanErr != nil {
				return scanErr
			}
			troubled = append(troubled, run)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("aiactivity: %w", err)
	}
	return troubled, nil
}
