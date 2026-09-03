// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetEmailPresentation serves the canonical email viewer's one read.
func (h Handlers) GetEmailPresentation(
	w http.ResponseWriter,
	r *http.Request,
	id crmcontracts.Id,
	params crmcontracts.GetEmailPresentationParams,
) {
	presentation, err := h.store.GetEmailPresentation(r.Context(),
		pathID[ids.ActivityKind](id), params.ThreadCursor)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, presentation)
}
