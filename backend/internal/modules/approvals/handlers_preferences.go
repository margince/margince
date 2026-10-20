// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func (h Handlers) ListApprovalPreferences(w http.ResponseWriter, r *http.Request) {
	preferences, err := h.svc.ApprovalPreferences(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ApprovalPreferences{Data: preferences})
}

func (h Handlers) UpdateApprovalPreference(w http.ResponseWriter, r *http.Request) {
	var request crmcontracts.ApprovalPreference
	if !httperr.Decode(w, r, &request) {
		return
	}
	if raw, sent := httperr.PresentField(r, "muted"); !sent || raw == nil {
		httperr.Write(w, r, httperr.Validation("muted", "required", "choose whether to hide this kind from your queue"))
		return
	}
	if raw, sent := httperr.PresentField(r, "kind"); !sent || raw == nil {
		httperr.Write(w, r, httperr.Validation("kind", "required", "name the kind of approval to set"))
		return
	}
	preferences, err := h.svc.SetApprovalPreference(r.Context(), request.Kind, request.Muted)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ApprovalPreferences{Data: preferences})
}
