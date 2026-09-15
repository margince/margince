// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactresearch

// The HTTP transport for the research surface. Wire concerns only: bind the
// path id, refuse the modes this surface cannot honestly serve, and hand the
// result to the sentinel error mapping.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated research stubs.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// RunContactResearch implements POST /contacts/{id}/research.
func (h Handlers) RunContactResearch(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	run, err := h.svc.Run(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, run)
}

// SaveContactResearch implements POST /contacts/{id}/research/save.
func (h Handlers) SaveContactResearch(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.SaveContactResearchRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	saved, err := h.svc.Save(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)), body)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Saved int `json:"saved"`
	}{Saved: saved})
}
