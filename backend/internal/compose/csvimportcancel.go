// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CancelImportRun ends a draft or a report nobody will approve and releases
// its uploaded file. Neither has written anything, so nothing is left to undo.
func (h importHandlers) CancelImportRun(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	if err := auth.Require(ctx, importRunGrant, principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	run, err := h.stagedFor(ctx, id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	cancelled, err := migration.NewRunStore(h.db).Cancel(ctx, run.ID)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractImportRun(cancelled))
}
