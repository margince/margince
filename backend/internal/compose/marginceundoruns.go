// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (h marginceChangeHandlers) undoRun(ctx context.Context, id ids.UUID, items []crmcontracts.UndoMarginceChangeItem) (crmcontracts.UndoMarginceChangesResult, error) {
	var out crmcontracts.UndoMarginceChangesResult
	if err := auth.RequireHuman(ctx); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := validateCorrectionUndoRun(id, items); err != nil {
		return out, err
	}
	if err := h.startCorrectionUndoRun(ctx, id, items); err != nil {
		return out, err
	}
	for {
		run, err := h.advanceCorrectionUndoRun(ctx, id)
		if err != nil {
			return out, err
		}
		if len(run.result.Results) == len(run.params.Items) {
			var visible crmcontracts.MarginceUndoRun
			err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
				var err error
				visible, err = visibleCorrectionUndoRun(ctx, tx, run)
				return err
			})
			if err != nil {
				return out, err
			}
			wireID := crmcontracts.Id(id)
			return crmcontracts.UndoMarginceChangesResult{RunId: &wireID, Complete: new(true), Results: visible.Results}, nil
		}
	}
}

func validateCorrectionUndoRun(id ids.UUID, items []crmcontracts.UndoMarginceChangeItem) error {
	if err := httperr.RequireBodyID("run_id", id); err != nil {
		return err
	}
	if len(items) == 0 || len(items) > 200 {
		return httperr.Validation("items", "out_of_range", "choose between 1 and 200 deals")
	}
	for _, item := range items {
		if err := httperr.RequireBodyID("items.deal_id", ids.UUID(item.DealId)); err != nil {
			return err
		}
		if len(item.AuditLogIds) == 0 || len(item.AuditLogIds) > 50 {
			return httperr.Validation("items.audit_log_ids", "out_of_range", "choose between 1 and 50 changes per deal")
		}
	}
	return nil
}

// A reversal and its result commit together, so a lost response can replay the result.
func (h marginceChangeHandlers) advanceCorrectionUndoRun(ctx context.Context, id ids.UUID) (correctionUndoRun, error) {
	var run correctionUndoRun
	err := storekit.RetryLockCycles(ctx, func() error {
		return database.WithWorkspaceSnapshot(ctx, h.pool, func(bound context.Context) error {
			return database.WithWorkspaceTx(bound, h.pool, func(tx pgx.Tx) error {
				var err error
				run, err = lockedCorrectionUndoRun(bound, tx, id)
				if err != nil || len(run.result.Results) >= len(run.params.Items) {
					return err
				}
				item := run.params.Items[len(run.result.Results)]
				result, err := h.undoBundle(storekit.WithBatch(bound, id), item)
				if err != nil {
					reason, refused := undoRefusal(err)
					if !refused {
						return err
					}
					result = refusedUndoItem(item, reason)
				}
				run.result.Results = append(run.result.Results, result)
				return writeCorrectionUndoResult(bound, tx, run)
			})
		})
	})
	return run, err
}

func refusedUndoItem(item crmcontracts.UndoMarginceChangeItem, reason crmcontracts.UndoMarginceChangeKeptReason) crmcontracts.UndoMarginceChangeResult {
	result := crmcontracts.UndoMarginceChangeResult{DealId: item.DealId, Outcome: crmcontracts.UndoMarginceChangeResultOutcomeRefused, Undone: []string{}, Kept: []crmcontracts.UndoMarginceChangeKept{}}
	for _, audit := range item.AuditLogIds {
		result.Kept = append(result.Kept, crmcontracts.UndoMarginceChangeKept{AuditLogId: audit, Reason: reason})
	}
	return result
}
