// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// phaseHistoryLinksDataTask links each project phase move written before the
// audit link existed to the audit row of its own transaction. The history can
// then name who made the move. A row with no single match stays unlinked and
// reads as unknown.
func phaseHistoryLinksDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792800500_phase_history_audit_links", Version: 1,
		Title: "Linking project phase moves to the record of who made them", Requires: []string{"core/1792800500"},
		Steps: []datatask.Step{{
			Name: "link_phase_history", Limit: 500, MinLimit: 10,
			Audit:    datatask.AuditPolicy{Reason: "projects.recordPhaseTransition stamps the same link in the transaction that writes the move, without a separate audit row"},
			Estimate: estimateUnlinkedPhaseHistory, Batch: linkPhaseHistoryBatch,
		}},
	}
}

func linkPhaseHistoryBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := projects.LinkPhaseHistoryTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Linked)
}

func estimateUnlinkedPhaseHistory(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := projects.EstimatePhaseHistoryLinks(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
