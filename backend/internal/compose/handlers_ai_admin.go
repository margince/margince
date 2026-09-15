// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

type aiAdminHandlers struct{ store *ai.AdminStore }

func (h aiAdminHandlers) GetAiBudget(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetAiBudget")
		return
	}
	out, err := h.store.ReadBudget(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h aiAdminHandlers) ReplaceAiBudget(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "ReplaceAiBudget")
		return
	}
	var change ai.BudgetChange
	if !httperr.Decode(w, r, &change) {
		return
	}
	out, err := h.store.ReplaceBudget(r.Context(), change)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h aiAdminHandlers) PreviewAiBudget(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "PreviewAiBudget")
		return
	}
	var change ai.BudgetChange
	if !httperr.Decode(w, r, &change) {
		return
	}
	out, err := h.store.PreviewBudget(r.Context(), change)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h aiAdminHandlers) GetAiStatus(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetAiStatus")
		return
	}
	out, err := h.store.ReadStatus(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h aiAdminHandlers) PreviewAiRouting(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "PreviewAiRouting")
		return
	}
	var next crmcontracts.AiRouting
	if !httperr.Decode(w, r, &next) {
		return
	}
	out, err := h.store.PreviewRouting(r.Context(), fromContractAiRouting(next))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
