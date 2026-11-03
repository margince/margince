// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const signalKindArg = "kind"

// PreviewSignalCarry reads the signals a merge would carry, worded for the
// reviewer. Nil words keep the stored summaries.
func PreviewSignalCarry(words SummaryWords) func(context.Context, pgx.Tx, string, ids.UUID) ([]crmcontracts.Signal, error) {
	return func(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.Signal, error) {
		return previewSignalCarryTx(ctx, tx, kind, source, words)
	}
}

// previewSignalCarryTx refuses an incomplete signal set rather than implying a partial merge scope.
func previewSignalCarryTx(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID, words SummaryWords) ([]crmcontracts.Signal, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM signal WHERE entity_type = @kind AND entity_id = @source ORDER BY id LIMIT 501`, pgx.StrictNamedArgs{signalKindArg: kind, "source": source})
	if err != nil {
		return nil, err
	}
	selected, err := pgx.CollectRows(rows, pgx.RowTo[ids.SignalID])
	if err != nil {
		return nil, err
	}
	if len(selected) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	out := make([]crmcontracts.Signal, 0, len(selected))
	for _, id := range selected {
		signal, err := ReadSignalTx(ctx, tx, id, storekit.IncludeArchived, words)
		if err != nil {
			return nil, err
		}
		out = append(out, signal)
	}
	return out, nil
}
