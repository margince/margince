// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

// The HTTP transport for the relationship brief. Wire concerns only: bind the
// path id, refuse the modes this read cannot honestly serve, and hand the
// result to the sentinel error mapping.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated contact-brief stubs.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetContactBrief implements GET /contacts/{id}/brief.
func (h Handlers) GetContactBrief(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serve(w, r, id, false)
}

// RegenerateContactBrief implements POST /contacts/{id}/brief — the explicit
// refresh behind the card's "outdated". It writes only the cached brief: no
// record field changes, and nothing is sent.
func (h Handlers) RegenerateContactBrief(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serve(w, r, id, true)
}

func (h Handlers) serve(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, force bool) {
	brief, err := h.svc.Get(r.Context(), ids.From[ids.ContactKind](ids.UUID(id)), force)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, brief)
}
