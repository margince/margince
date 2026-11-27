// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (h Handlers) RenameConsentPurpose(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var in crmcontracts.RenameConsentPurposeJSONRequestBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	purpose, err := h.store.RenamePurpose(r.Context(), pathID[ids.PurposeKind](id), in.Label)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wirePurpose(purpose))
}

func (h Handlers) ArchiveConsentPurpose(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.ArchivePurpose(r.Context(), pathID[ids.PurposeKind](id)); err != nil {
		writeConsentErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
