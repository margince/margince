// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package project360

// The HTTP transport for the project page. Wire concerns only: bind the
// path id, refuse the mode this read cannot honestly serve, and hand the
// result to the sentinel error mapping. The service owns the transaction
// and every gate.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated GetProject360 stub.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetProject360 implements GET /projects/{id}/360.
func (h Handlers) GetProject360(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	view, err := h.svc.Assemble(r.Context(), ids.From[ids.ProjectKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, view)
}
