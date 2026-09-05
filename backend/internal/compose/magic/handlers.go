// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The route: one read, no verbs.
//
// Nothing is answered from here. A pending approval is decided where approvals
// are decided; an undo calls the record's own restore route. This surface
// reports, and the surfaces that own each verb keep it — a second place to
// answer a decision is a second place for the two answers to disagree.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Handlers binds the route to the read.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the route.
func NewHandlers(svc *Service) Handlers { return Handlers{svc: svc} }

// GetMagic answers what the machinery did, needs, could not finish, and is
// watching.
func (h Handlers) GetMagic(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetMagicParams,
) {
	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	out, err := h.svc.Read(r.Context(), params.Since, limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
