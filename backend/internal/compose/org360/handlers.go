// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The HTTP transport for the company record page. Wire concerns only:
// bind the path id, refuse the modes this read cannot honestly serve, and
// hand the result to the sentinel error mapping. The service owns the
// transaction and every gate.

import (
	"context"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OverlayMode answers whether the calling workspace reads from an
// incumbent mirror instead of this system of record. The composition
// layer injects the one Dispatcher every other overlay-aware read uses,
// so a mode flip is observed here at the same moment it is observed there.
type OverlayMode func(ctx context.Context) (bool, error)

// Handlers shadows the generated GetOrganization360 /
// AcknowledgeOrganizationView stubs.
type Handlers struct {
	svc     *Service
	overlay OverlayMode
	// roleLane is the propose-roles model lane, or nil in a process role that
	// wired none. Held here rather than on the Service because it is the only
	// part of this composite that calls a model at all: every other section is
	// a read, and giving the whole service a lane would suggest otherwise.
	roleLane Completer
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service, overlay OverlayMode) Handlers {
	return Handlers{svc: svc, overlay: overlay}
}

// WithRoleLane binds the model lane that reads buying roles.
//
// Optional by design: without it ProposeDealRoles answers 501, which is the
// honest answer for a role there is no non-guessing way to read.
func (h Handlers) WithRoleLane(lane Completer) Handlers {
	h.roleLane = lane
	return h
}

// ProposeDealRoles implements POST /deals/{id}/role-proposals.
func (h Handlers) ProposeDealRoles(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.nativeOnly(w, r) {
		return
	}
	if h.roleLane == nil {
		httperr.NotImplemented(w, r, "ProposeDealRoles (no model path configured)")
		return
	}
	out, err := h.svc.ProposeRoles(r.Context(), h.roleLane, ids.From[ids.DealKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetOrganization360 implements GET /organizations/{id}/360.
func (h Handlers) GetOrganization360(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetOrganization360Params) {
	if !h.nativeOnly(w, r) {
		return
	}
	var opts AssembleOptions
	if params.ProjectId != nil {
		projectID := ids.From[ids.ProjectKind](ids.UUID(*params.ProjectId))
		opts.ProjectID = &projectID
	}
	view, err := h.svc.AssembleScoped(r.Context(), ids.From[ids.OrganizationKind](ids.UUID(id)), opts)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, view)
}

// GetOrganizationGraph implements GET /organizations/{id}/graph.
func (h Handlers) GetOrganizationGraph(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.nativeOnly(w, r) {
		return
	}
	graph, err := h.svc.Graph(r.Context(), ids.From[ids.OrganizationKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, graph)
}

// GetOrganizationCoverage implements GET /organizations/{id}/coverage.
func (h Handlers) GetOrganizationCoverage(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.nativeOnly(w, r) {
		return
	}
	coverage, err := h.svc.Coverage(r.Context(), ids.From[ids.OrganizationKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, coverage)
}

// ListOrganizationContacts implements GET /organizations/{id}/contacts.
func (h Handlers) ListOrganizationContacts(w http.ResponseWriter, r *http.Request, id crmcontracts.Id,
	params crmcontracts.ListOrganizationContactsParams,
) {
	if !h.nativeOnly(w, r) {
		return
	}
	// A value the enum never declared is refused rather than ignored. An
	// unrecognised status silently matched nothing and answered 200 with an
	// empty page, which reads as "this account has no such contacts" — the
	// wrong answer to a typo, and indistinguishable from the right one.
	q := ContactListQuery{
		Query:  params.Q,
		Cursor: params.Cursor,
		Limit:  params.Limit,
		Sort:   string(crmcontracts.Recommended),
	}
	if params.Status != nil {
		if !params.Status.Valid() {
			httperr.Write(w, r, httperr.Validation("status", "invalid_enum",
				"status is answered, no_reply or untried"))
			return
		}
		status := people.Engagement(*params.Status)
		q.Status = &status
	}
	if params.Sort != nil {
		if !params.Sort.Valid() {
			httperr.Write(w, r, httperr.Validation("sort", "invalid_enum",
				"sort is recommended, -last_interaction, -strength or name"))
			return
		}
		q.Sort = string(*params.Sort)
	}
	page, err := h.svc.ContactPage(r.Context(), ids.From[ids.OrganizationKind](ids.UUID(id)), q)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, page)
}

// AcknowledgeOrganizationView implements POST /organizations/{id}/view-ack.
func (h Handlers) AcknowledgeOrganizationView(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.nativeOnly(w, r) {
		return
	}
	ack, err := h.svc.Acknowledge(r.Context(), ids.From[ids.OrganizationKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, ack)
}

// DismissOrganizationSuggestion implements
// POST /organizations/{id}/suggestions/dismiss.
func (h Handlers) DismissOrganizationSuggestion(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if !h.nativeOnly(w, r) {
		return
	}
	var req crmcontracts.DismissOrganizationSuggestionJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.svc.DismissSuggestion(r.Context(),
		ids.From[ids.OrganizationKind](ids.UUID(id)), req.Fingerprint); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// nativeOnly refuses an overlay-mode workspace. The mirror holds the
// incumbent's records, not our relationship edges, tags, approvals or
// visit marks, so there is no honest 360 to assemble from it — the same
// refusal entity-scoped activity reads already give, rather than a page
// that quietly omits most of itself. A mode-resolution failure refuses
// too: serving native data because the lookup broke is the silent
// fallback the overlay module exists to prevent.
func (h Handlers) nativeOnly(w http.ResponseWriter, r *http.Request) bool {
	overlay, err := h.overlay(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return false
	}
	if overlay {
		httperr.Write(w, r, httperr.Validation("id", "unsupported_in_overlay_mode",
			"the company view is assembled from this system of record; while the workspace reads from the incumbent mirror, open the account in the incumbent's own UI"))
		return false
	}
	return true
}
