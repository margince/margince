// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// An agent on a colleague's passport now runs what used to wait for a human. The
// approvals such an agent staged before that rule still wait. The agent's retry
// has already run the call, so approving one later would run it again. These
// close them.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// liftedAgentCallReason is the audit reason on a withdrawn agent call.
const liftedAgentCallReason = "an agent on its colleague's passport now runs this without an approval"

// cursorArg names the id a withdrawal batch resumes after.
const cursorArg = "after"

// capArg names the bound an estimate counts up to.
const capArg = "cap"

// batchLimitArg names the size of a withdrawal batch.
const batchLimitArg = "batch_limit"

// liftedAgentCallScope selects the live approvals a passport staged for a tool
// kind that no longer waits. One a scheduled run is paused on stays: the run
// resumes only on a decision, which a withdrawal never sends.
const liftedAgentCallScope = `FROM approval WHERE status = 'pending' AND expires_at > now()
	AND passport_id IS NOT NULL AND kind = ANY(@kinds)
	AND NOT EXISTS (SELECT 1 FROM agent_run r WHERE r.approval_id = approval.id AND r.status = 'awaiting_approval')`

// EstimateLiftedAgentCalls bounds the withdrawal task's work for these kinds.
func EstimateLiftedAgentCalls(kinds []string) func(context.Context, datatask.Querier, int64) (int64, bool, error) {
	return func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
		if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
			return 0, false, err
		}
		var count int64
		err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id `+liftedAgentCallScope+` LIMIT @cap) candidates`,
			pgx.NamedArgs{"kinds": kinds, capArg: maximum + 1}).Scan(&count)
		return min(count, maximum), count > maximum, err
	}
}

// WithdrawLiftedAgentCallsTx closes each pending approval a passport staged for
// one of these kinds. The withdrawal is audited and sends no event, as the
// promise withdrawal does.
func WithdrawLiftedAgentCallsTx(kinds []string) func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
	return func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		return withdrawPendingBatch(ctx, tx, in, pendingWithdrawal{
			scope: liftedAgentCallScope, args: pgx.NamedArgs{"kinds": kinds},
			noun: "agent calls", reason: liftedAgentCallReason,
		})
	}
}

// pendingWithdrawal names the pending approvals one data task withdraws.
type pendingWithdrawal struct {
	// scope is the clause selecting them, and args binds it.
	scope string
	args  pgx.NamedArgs
	// noun names them in a read failure.
	noun string
	// reason is the audit reason on each withdrawal.
	reason string
}

// withdrawPendingBatch withdraws one id-ordered batch through the approvals
// writer, which audits each one.
func withdrawPendingBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, pending pendingWithdrawal) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	pending.args[cursorArg], pending.args[batchLimitArg] = after, in.Limit
	rows, err := tx.Query(ctx, `SELECT id `+pending.scope+` AND id > @after ORDER BY id LIMIT @batch_limit`, pending.args)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("approvals: reading pending %s: %w", pending.noun, err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, id := range scanned {
		withdrawn, err := withdrawInTx(ctx, tx, ids.From[ids.ApprovalKind](id), pending.reason)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if withdrawn {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}
