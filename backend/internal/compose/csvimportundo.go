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
	writers, err := newBundleWriters(ctx, h.db, run.ID, run.Mapping, nil)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// Moved to undoing before the answer, so a refused undo is still refused
	// here; the reversal runs detached and holds its own claim.
	if err := runs.ClaimUndo(ctx, run.ID); err != nil {
		httperr.Write(w, r, h.sessionRefusal(ctx, err))
		return
	}
	h.runDetached(ctx, run.ID, "undo", func(undoCtx context.Context) error {
		_, err := runs.Undo(undoCtx, run.ID, writers)
		return err
	})
	final, err := runs.GetStaged(ctx, run.ID)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, toContractImportRun(final))
}
