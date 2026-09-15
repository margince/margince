// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

// The HTTP transport for the account scan. Wire concerns only: bind the path
// id, say whether this is a read or an ensure, and hand the result to the
// sentinel error mapping. The service owns every other gate.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated scan stubs.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetCompanyScan implements GET /companies/{id}/scan.
func (h Handlers) GetCompanyScan(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	scan, err := h.svc.Get(r.Context(), ids.From[ids.CompanyKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, scan)
}

// EnsureCompanyScan implements POST /companies/{id}/scan. The body
// is optional: an open with nothing to say sends none.
func (h Handlers) EnsureCompanyScan(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.CompanyScanRequest
	if r.ContentLength != 0 && !httperr.Decode(w, r, &req) {
		return
	}
	force := req.Force != nil && *req.Force
	scan, err := h.svc.Ensure(r.Context(), ids.From[ids.CompanyKind](ids.UUID(id)), force)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, scan)
}
