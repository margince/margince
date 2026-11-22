// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The kinds of work done on a deal that DealActionsBetweenTx reports.
const (
	DealActionTaskDone    = "task_done"
	DealActionMailSent    = "mail_sent"
	DealActionMeetingHeld = "meeting_held"
)

// DealAction is one thing done on a deal inside a window.
type DealAction struct {
	DealID     ids.UUID
	ActivityID ids.UUID
	Kind       string
	At         time.Time
	Subject    string
}

// DealActionsBetweenTx lists the tasks finished, mails sent and meetings held on
// these deals from `after` up to but not including `until`, oldest first.
// It reads the activity rows the write shape keeps.
// Only rows the caller may read count: the deal's row scope and the activity's audience both apply.
func DealActionsBetweenTx(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, after, until time.Time) ([]DealAction, error) {
	if len(dealIDs) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	deals, err := readableDealBatchSQL(ctx, dealIDs, arg)
	if err != nil {
		return nil, err
	}
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if content == "" {
		content = sqlTrue
	}
	from, to := arg(after), arg(until)
	taskWindow := fmt.Sprintf("a.done_at >= $%d AND a.done_at < $%d", from, to)
	happenedWindow := occurredday.WithinSQL("a", fmt.Sprintf("$%d", from), fmt.Sprintf("$%d", to))
	// The moment each kind counts at differs: a task when it was ticked, a mail
	// and a meeting when they happened.
	rows, err := tx.Query(ctx, `
		SELECT d.id, a.id,
		       CASE WHEN a.kind = 'task' THEN 'task_done' WHEN a.kind = 'email' THEN 'mail_sent' ELSE 'meeting_held' END,
		       CASE WHEN a.kind = 'task' THEN a.done_at ELSE a.occurred_at END AS at,
		       coalesce(a.subject, '')
		  FROM deal d
		  JOIN activity_link l ON l.deal_id = d.id
		  JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
		 WHERE `+deals+` AND `+content+` AND (
		       (a.kind = 'task' AND a.is_done AND `+taskWindow+`)
		    OR (a.kind = 'email' AND a.direction = 'outbound' AND `+happenedWindow+`)
		    OR (a.kind = 'meeting' AND (a.meeting_status IS NULL OR a.meeting_status = 'held') AND `+happenedWindow+`))
		 ORDER BY at, a.id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (DealAction, error) {
		var out DealAction
		err := row.Scan(&out.DealID, &out.ActivityID, &out.Kind, &out.At, &out.Subject)
		return out, err
	})
}
