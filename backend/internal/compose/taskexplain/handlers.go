// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package taskexplain

import (
	"net/http"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated task-explanation stub.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetTaskExplanation implements GET /activities/{id}/task-explanation.
func (h Handlers) GetTaskExplanation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	explanation, err := h.svc.Get(r.Context(), ids.From[ids.ActivityKind](ids.UUID(id)))
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, explanation)
}
