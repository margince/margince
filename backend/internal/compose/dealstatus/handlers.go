// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers serves the card.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to the service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetDealStatus returns the card, writing it when the cached one is stale. It
// is a read: the only row it writes is its own cache entry, which is derived
// content and carries no audit or outbox row.
func (h Handlers) GetDealStatus(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetDealStatusParams,
) {
	refresh := params.Refresh != nil && *params.Refresh
	out, err := h.svc.Get(r.Context(), ids.From[ids.DealKind](ids.UUID(id)), refresh)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
