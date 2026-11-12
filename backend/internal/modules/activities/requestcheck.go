// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UncheckableRequestReminders answers which of the named open request reminders
// the settlement pass will never judge. Those are the requests
// settlementEvidenceCompleteSQL keeps off that pass's list. Each names our
// newest reply among the answers that pass would read.
func (s *Store) UncheckableRequestReminders(ctx context.Context, tasks []ids.UUID, asOf time.Time) (map[ids.UUID]crmcontracts.TaskRequestCheck, error) {
	out := map[ids.UUID]crmcontracts.TaskRequestCheck{}
	err := auth.Require(ctx, "activity", principal.ActionRead)
	if errors.Is(err, apperrors.ErrPermissionDenied) || (err == nil && len(tasks) == 0) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		held, err := uncheckableReminders(ctx, tx, tasks, asOf)
		if err != nil || len(held) == 0 {
			return err
		}
		for _, task := range held {
			out[task] = crmcontracts.TaskRequestCheck{State: crmcontracts.TaskRequestCheckStateTextNotChecked}
		}
		return nameOurNewestReplies(ctx, tx, held, asOf, out)
	})
	return out, err
}

// uncheckableReminders keeps the open reminders the reader may read whose
// request mail was never read in full. Each statement carries the content
// gate once, because Postgres plans every copy and the pool keeps that memory.
func uncheckableReminders(ctx context.Context, tx pgx.Tx, tasks []ids.UUID, asOf time.Time) ([]ids.UUID, error) {
	args := []any{asOf, tasks}
	arg := func(v any) int { args = append(args, v); return len(args) }
	readable, err := contentOrTrue(ctx, "task", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT task.id
		  FROM activity task
		  JOIN activity a ON a.id = task.source_activity_id
		 WHERE task.id = ANY($2) AND task.kind = 'task' AND NOT task.is_done
		   AND task.archived_at IS NULL AND task.source_system = '%s' AND (%s)
		   AND NOT (%s)`,
		EmailRequestTaskSource, readable, settlementEvidenceCompleteSQL), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading which request reminders cannot be checked: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading which request reminders cannot be checked: %w", err)
	}
	return held, nil
}

// nameOurNewestReplies adds, per reminder, the newest mail we sent that
// answers its request and that the reader may open.
func nameOurNewestReplies(ctx context.Context, tx pgx.Tx, tasks []ids.UUID, asOf time.Time, out map[ids.UUID]crmcontracts.TaskRequestCheck) error {
	args := []any{asOf, tasks}
	arg := func(v any) int { args = append(args, v); return len(args) }
	sentReadable, err := contentOrTrue(ctx, "sent", arg)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT task.id, answer.id, answer.occurred_at, answer.occurred_precision
		  FROM activity task
		  JOIN activity a ON a.id = task.source_activity_id
		  JOIN activity answer ON answer.id = (%s)
		 WHERE task.id = ANY($2)`, ourNewestReply(sentReadable)), args...)
	if err != nil {
		return fmt.Errorf("activities: reading the reply a reminder names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var task, answer ids.UUID
		var at time.Time
		var precision string
		if err := rows.Scan(&task, &answer, &at, &precision); err != nil {
			return fmt.Errorf("activities: reading the reply a reminder names: %w", err)
		}
		check := out[task]
		check.OurLastReply = &crmcontracts.TaskRequestCheckReply{
			ActivityId: openapi_types.UUID(answer), OccurredAt: at, OccurredPrecision: crmcontracts.OccurredPrecision(precision),
		}
		out[task] = check
	}
	return rows.Err()
}

// ourNewestReply is the newest answer that is our attested mail and that the
// reader may read. A call, a meeting or the customer's own mail is no reply.
func ourNewestReply(readable string) string {
	return newestAnswer("sent.id", `JOIN activity sent ON sent.id = newest.id AND sent.kind = 'email'
        AND `+correspondence.OurOutboundSQL("sent")+` AND (`+readable+`)`)
}

func contentOrTrue(ctx context.Context, alias string, arg func(any) int) (string, error) {
	content, err := auth.ActivityContentClause(ctx, alias, arg)
	if content == "" {
		content = sqlTrue
	}
	return content, err
}
