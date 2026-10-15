// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
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
		httperr.Write(w, r, migration.RefuseRun(run.ID, "this import carries no mapping, so it created nothing to undo"))
		return
	}

	queue, ws, err := h.jobQueue(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// Built before the move, so an undo whose writers cannot be built is
	// refused while the run is still complete.
	if _, err := newBundleWriters(ctx, h.db, run.ID, run.Mapping, nil); err != nil {
		httperr.Write(w, r, err)
		return
	}
	undoing, err := migration.NewRunStore(h.db).ClaimUndo(ctx, run.ID, queue.undos(ws))
	if err != nil {
		httperr.Write(w, r, h.sessionRefusal(ctx, err))
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, toContractImportRun(undoing))
}
