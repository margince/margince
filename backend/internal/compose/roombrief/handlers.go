// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"net/http"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated room-brief stub.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetDealRoomBrief implements GET /deal-rooms/{id}/brief.
func (h Handlers) GetDealRoomBrief(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	brief, err := h.svc.Get(r.Context(), ids.From[ids.DealRoomKind](ids.UUID(id)))
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, brief)
}
