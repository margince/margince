// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// retiredStageCardKind is the stage-move card Margince no longer stages or
// applies: it moves the deal itself, with an Undo.
const retiredStageCardKind = "stage_progression"

// retiredStageCardReason is the audit reason on an expired stage-move card.
const retiredStageCardReason = "Margince now moves a deal itself, with an Undo, so this card is no longer asked"

const retiredStageCardScope = `FROM approval WHERE kind = @kind AND status = 'pending'`

// EstimateRetiredStageCards bounds the expiry task's work.
func EstimateRetiredStageCards(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id `+retiredStageCardScope+` LIMIT @cap) candidates`,
		pgx.NamedArgs{approvalKeyKind: retiredStageCardKind, capArg: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

// ExpireRetiredStageCardsTx expires each pending stage-move card through the
// approvals writer, which audits the expiry.
func ExpireRetiredStageCardsTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	return withdrawPendingBatch(ctx, tx, in, pendingWithdrawal{
		scope: retiredStageCardScope, args: pgx.NamedArgs{approvalKeyKind: retiredStageCardKind},
		noun: "stage-move cards", reason: retiredStageCardReason,
	})
}
