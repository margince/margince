// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (h marginceChangeHandlers) ReadMarginceUndoRuns(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.Require(r.Context(), "deal", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var request crmcontracts.MarginceUndoRunsRequest
	if !httperr.Decode(w, r, &request) {
		return
	}
	if len(request.RunIds) == 0 || len(request.RunIds) > 50 {
		httperr.Write(w, r, httperr.Validation("run_ids", "out_of_range", "choose between 1 and 50 saved runs"))
		return
	}
	named := make([]ids.UUID, 0, len(request.RunIds))
	for _, id := range request.RunIds {
		named = append(named, ids.UUID(id))
	}
	runs, err := h.readCorrectionUndoRuns(r.Context(), named)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.MarginceUndoRuns{Runs: runs})
}

func (h marginceChangeHandlers) readCorrectionUndoRuns(ctx context.Context, named []ids.UUID) ([]crmcontracts.MarginceUndoRun, error) {
	out := make([]crmcontracts.MarginceUndoRun, 0, len(named))
	err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT "+correctionUndoColumns+` FROM bulk_operation WHERE id=ANY(@ids::uuid[]) AND verb=@verb ORDER BY created_at,id`, pgx.NamedArgs{"ids": named, "verb": correctionUndoVerb})
		if err != nil {
			return err
		}
		saved, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (correctionUndoRun, error) { return scanCorrectionUndoRun(row) })
		if err != nil {
			return err
		}
		for _, run := range saved {
			if !mayReadBatch(ctx, run.requester) {
				continue
			}
			visible, err := visibleCorrectionUndoRun(ctx, tx, run)
			if err != nil {
				return err
			}
			out = append(out, visible)
		}
		return nil
	})
	return out, err
}

func visibleCorrectionUndoRun(ctx context.Context, tx pgx.Tx, run correctionUndoRun) (crmcontracts.MarginceUndoRun, error) {
	var named []ids.UUID
	for _, item := range run.params.Items {
		named = append(named, ids.UUID(item.DealId))
	}
	seen, err := visibleRecords(ctx, tx, crmcontracts.BulkRecordTypeDeal, named)
	if err != nil {
		return crmcontracts.MarginceUndoRun{}, err
	}
	out := crmcontracts.MarginceUndoRun{RunId: crmcontracts.Id(run.id), CreatedAt: run.createdAt,
		Complete: len(run.result.Results) == len(run.params.Items), Items: []crmcontracts.UndoMarginceChangeItem{}, Results: []crmcontracts.UndoMarginceChangeResult{}}
	for _, item := range run.params.Items {
		if seen[ids.UUID(item.DealId)] {
			out.Items = append(out.Items, item)
		}
	}
	for _, result := range run.result.Results {
		if !seen[ids.UUID(result.DealId)] {
			continue
		}
		out.Results = append(out.Results, result)
	}

	return out, nil
}
