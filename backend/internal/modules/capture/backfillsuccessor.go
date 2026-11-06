// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func rescheduleBackfillTx(ctx context.Context, tx pgx.Tx, id ids.UUID, delay time.Duration) error {
	execution, admitted := ctx.Value(backfillExecutionKeyType{}).(backfillExecution)
	if !admitted {
		return nil
	}
	args := []any{id}
	idArg := len(args)
	args = append(args, execution.generation)
	generationArg := len(args)
	args = append(args, delay.Seconds())
	delayArg := len(args)
	query := fmt.Sprintf(`UPDATE capture_backfill SET execution_generation=execution_generation+1,
 execution_owner_pid=NULL,execution_due_at=now()+make_interval(secs=>$%d)
 WHERE id=$%d AND execution_generation=$%d AND status='running'
 RETURNING execution_generation,execution_due_at`, delayArg, idArg, generationArg)
	var generation int64
	var due time.Time
	if err := tx.QueryRow(ctx, query, args...).Scan(&generation, &due); err != nil {
		return err
	}
	return execution.scheduler.Enqueue(ctx, tx, id, generation, due)
}

func markBackfillScheduled(ctx context.Context) {
	if execution, admitted := ctx.Value(backfillExecutionKeyType{}).(backfillExecution); admitted {
		*execution.scheduled = true
	}
}

const clearBackfillAdmission = `, admission_slot = NULL, execution_owner_pid = NULL,
 execution_generation = execution_generation + 1, execution_due_at = NULL, backoff_is_mailbox_scoped = false`

func mailboxScopedBackoff(cause error) bool {
	var limited *connector.RateLimitedError
	return errors.As(cause, &limited) && limited.MailboxScoped()
}
