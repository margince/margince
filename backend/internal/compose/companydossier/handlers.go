// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

// The HTTP transport for the company dossier. Wire concerns only: bind the path
// id, say whether this is a read or an explicit refresh, and hand the result to
// the sentinel error mapping. The service owns every other gate.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers shadows the generated dossier and growth-fit stubs.
type Handlers struct {
	svc       *Service
	growthFit *GrowthFitService
}

// NewHandlers binds the transport to ready services; compose constructs it once
// per process role.
func NewHandlers(svc *Service, growthFit *GrowthFitService) Handlers {
	return Handlers{svc: svc, growthFit: growthFit}
}

// GetCompanyDossier implements GET /companies/{id}/dossier.
func (h Handlers) GetCompanyDossier(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serve(w, r, id, false)
}

// RefreshCompanyDossier implements POST /companies/{id}/dossier — the
// explicit rebuild, past a fingerprint that still matches.
func (h Handlers) RefreshCompanyDossier(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serve(w, r, id, true)
}

// GetCompanyGrowthFit implements GET /companies/{id}/growth-fit.
func (h Handlers) GetCompanyGrowthFit(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serveGrowthFit(w, r, id, false)
}

// RefreshCompanyGrowthFit implements POST /companies/{id}/growth-fit —
// the caller's own re-assessment, past a fingerprint that still matches.
func (h Handlers) RefreshCompanyGrowthFit(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	h.serveGrowthFit(w, r, id, true)
}

// GetClaimEvidence implements GET
// /companies/{id}/evidence/{entityType}/{entityId} — the receipt behind one
// record a generated sentence cited.
//
// It is the affordance that makes the prose above it worth reading: a claim the
// reader can open is one they can disagree with.
func (h Handlers) GetClaimEvidence(w http.ResponseWriter, r *http.Request,
	id crmcontracts.Id, entityType string, entityID openapi_types.UUID,
) {
	// A growth fit or dossier is for a contact; an agent holding a passport has
	// the records themselves and needs no receipt for them.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The receipt's labels follow the installation's base language, the same
	// source the dossier itself is written from — a receipt opened off a German
	// card must not answer in English.
	receipt, err := EvidenceFor(r.Context(), h.svc.facts,
		ids.From[ids.CompanyKind](ids.UUID(id)), entityType, entityID,
		identity.BaseLanguageForPrompt(r.Context(), h.svc.pool))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, receipt)
}

func (h Handlers) serve(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, force bool) {
	dossier, err := h.svc.Get(r.Context(), ids.From[ids.CompanyKind](ids.UUID(id)), force)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, dossier)
}

func (h Handlers) serveGrowthFit(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, force bool) {
	fit, err := h.growthFit.Get(r.Context(), ids.From[ids.CompanyKind](ids.UUID(id)), force)
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, fit)
}
