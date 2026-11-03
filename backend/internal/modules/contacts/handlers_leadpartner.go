// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AcceptPartnerLead accepts a lead for the partner that referred it.
func (h Handlers) AcceptPartnerLead(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.AcceptPartnerLeadParams) {
	out, err := h.store.AcceptPartnerLead(r.Context(), pathID[ids.LeadKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
