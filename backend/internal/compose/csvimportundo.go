// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// UndoImportRun reverses a completed csv run (IEM-WIRE-9). Mapping.Object is
// read from the run rather than the request: the run itself is the only
// authority for what its own rows are.
func (h importHandlers) UndoImportRun(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	run, err := h.staged(r, id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if run.Mapping == nil {
		httperr.Write(w, r, fmt.Errorf("import run %s carries no mapping, so it created nothing to undo: %w", run.ID, apperrors.ErrConflict))
		return
	}

	runs := migration.NewRunStore(h.db)
	writers := newCSVWriters(h.db, run.ID, run.Mapping)
	// The reversal outlives the request deliberately, the same reason the
	// commit does (ApproveImportRun): cancelling it when the browser goes
	// away must not leave the run `undoing` with rows already reversed and
	// nothing able to record how far it got.
	undoCtx, releaseUndo := context.WithTimeout(context.WithoutCancel(ctx), importCommitTimeout)
	defer releaseUndo()
	if _, err := runs.Undo(undoCtx, run.ID, writers); err != nil {
		httperr.Write(w, r, err)
		return
	}

	final, err := runs.GetStaged(undoCtx, run.ID)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, toContractImportRun(final))
}
