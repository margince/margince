// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft

// The HTTP transport. Wire concerns only: bind the path id, decode the optional
// body, and hand the result to the sentinel error
// mapping. The service owns every gate that matters.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated DraftLeadEmail stub.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service; compose constructs it
// once per process role.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// DraftLeadEmail implements POST /leads/{id}/draft-email.
func (h Handlers) DraftLeadEmail(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	req, ok := decodeRequest(w, r)
	if !ok {
		return
	}
	draft, err := h.svc.Draft(r.Context(), ids.From[ids.LeadKind](ids.UUID(id)), req)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, draft)
}

// decodeRequest reads the optional steering. An absent body is the ordinary
// case — "write to this lead" with nothing further to say — so it is not a
// missing field.
func decodeRequest(w http.ResponseWriter, r *http.Request) (Request, bool) {
	if r.ContentLength == 0 {
		return Request{}, true
	}
	var body crmcontracts.DraftLeadEmailJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return Request{}, false
	}
	var req Request
	if body.Intent != nil {
		req.Intent = *body.Intent
	}
	return req, true
}
