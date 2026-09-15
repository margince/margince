// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// The HTTP transport for the account brief. Wire concerns only: bind the
// path id, say whether this is a read or an explicit refresh, and hand the
// result to the sentinel error mapping. The service owns every gate.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated GetCompanyBrief /
// RegenerateCompanyBrief stubs.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service; compose constructs it
// once per process role.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// GetCompanyBrief implements GET /companies/{id}/brief.
func (h Handlers) GetCompanyBrief(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetCompanyBriefParams) {
	h.serve(w, r, id, false, projectScope(params.ProjectId))
}

// RegenerateCompanyBrief implements POST /companies/{id}/brief —
// the explicit refresh behind "outdated — refresh".
func (h Handlers) RegenerateCompanyBrief(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.RegenerateCompanyBriefParams) {
	h.serve(w, r, id, true, projectScope(params.ProjectId))
}

// projectScope binds the optional query narrowing. The generated parameter
// type is the bare uuid alias, so the kind is put back here.
func projectScope(raw *openapi_types.UUID) *ids.ProjectID {
	if raw == nil {
		return nil
	}
	id := ids.From[ids.ProjectKind](ids.UUID(*raw))
	return &id
}

// AskAboutCompany implements POST /companies/{id}/ask.
func (h Handlers) AskAboutCompany(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.AskAboutCompanyJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	answer, err := h.svc.AskScoped(r.Context(), ids.From[ids.CompanyKind](ids.UUID(id)), req.Question, projectScope(req.ProjectId))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, answer)
}

func (h Handlers) serve(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, force bool, projectID *ids.ProjectID) {
	brief, err := h.svc.GetScoped(r.Context(), ids.From[ids.CompanyKind](ids.UUID(id)), force, projectID)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, brief)
}
