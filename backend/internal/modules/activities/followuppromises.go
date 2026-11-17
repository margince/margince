// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// FollowUpPromise is one open task of the reader's derived from an email.
type FollowUpPromise struct {
	ID, Email ids.UUID
	// DueAt is the instant the task lane orders by; nil when undated.
	DueAt *time.Time
}

// followUpPromisesSQL names the reader's open promises of the emails. The
// literal IS NOT NULL keeps idx_activity_source_activity under any plan.
func followUpPromisesSQL(emails, reader string) string {
	return `SELECT promise.id, promise.source_activity_id, ` + storekit.TaskDueSQL("promise") + `
	  FROM activity promise
	 WHERE promise.source_activity_id IS NOT NULL AND promise.source_activity_id = ANY (` + emails + `::uuid[])
	   AND promise.kind = 'task' AND NOT promise.is_done AND ` + readersPromiseSQL("promise", reader)
}

// FollowUpPromises names the reader's own open tasks derived from any of these
// emails. It answers ids and due dates only: a caller draws a task through the
// task lane's own read, which holds the Worklist's rules for it.
func (s *Store) FollowUpPromises(ctx context.Context, emails []ids.UUID) ([]FollowUpPromise, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	reader := readerOrNobody(ctx)
	if reader.IsZero() || len(emails) == 0 {
		return nil, nil
	}
	var out []FollowUpPromise
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) string { args = append(args, v); return sprintf("$%d", len(args)) }
		query := followUpPromisesSQL(arg(emails), arg(reader))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (FollowUpPromise, error) {
			var promise FollowUpPromise
			err := row.Scan(&promise.ID, &promise.Email, &promise.DueAt)
			return promise, err
		})
		return err
	})
	return out, err
}
