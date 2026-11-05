// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
		pgx.NamedArgs{"kind": retiredStageCardKind, capArg: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

// ExpireRetiredStageCardsTx expires each pending stage-move card through the
// approvals writer, which audits the expiry.
func ExpireRetiredStageCardsTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id `+retiredStageCardScope+` AND id > @after ORDER BY id LIMIT @batch_limit`,
		pgx.NamedArgs{"kind": retiredStageCardKind, cursorArg: after, "batch_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("approvals: reading pending stage-move cards: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, id := range candidates {
		expired, err := withdrawInTx(ctx, tx, ids.From[ids.ApprovalKind](id), retiredStageCardReason)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if expired {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, candidates, changed)
}
