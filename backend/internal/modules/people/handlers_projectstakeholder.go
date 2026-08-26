// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The project-stakeholder surface. It lives here, not with the project
// record, because `relationship` is this module's table: a project
// stakeholder is the deal-stakeholder edge pointed at a body of work
// instead of a deal, and reusing the edge is what makes "which projects
// is this person accountable for" a query rather than a note.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ProjectStakeholderKind is the edge kind this surface reads and writes;
// projectObjectName is the RBAC object and visibility-probe table the edge
// annotates. Both spelled once so the kind, the anchor and the probe cannot
// drift apart.
const (
	ProjectStakeholderKind = "project_stakeholder"
	projectObjectName      = "project"
	// projectStakeholderUnique is the index that makes "already attached"
	// detectable rather than duplicated. Named here because three places rely
	// on the same spelling: the constraint mapper, its client-facing detail,
	// and the attach path that recovers from losing its own race.
	projectStakeholderUnique = "uq_rel_project_stakeholder"

	// ProjectCompanyKind is the edge naming a company that is ON a project. A
	// project is work several companies do together — a customer, a partner, a
	// subcontractor — so the companies are edges rather than one anchor column,
	// and they ride this table for the same reason the stakeholders do: it
	// already carries a role, a source and the archive semantics an edge needs.
	ProjectCompanyKind = "project_company"
)

// ListProjectStakeholders serves the project-scoped stakeholder view; the
// project itself must be visible (the endpoint-scope rule then re-applies
// per edge).
func (h Handlers) ListProjectStakeholders(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	projectID := pathID[ids.ProjectKind](id)
	kind := ProjectStakeholderKind
	rels, page, err := h.store.ListRelationships(r.Context(), ListRelationshipsInput{
		Kind:      &kind,
		ProjectID: &projectID,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if len(rels) == 0 {
		// Distinguish "no stakeholders" from "no such project" without
		// leaking: the project read carries its own row scope.
		if err := h.store.EnsureProjectVisible(r.Context(), projectID); err != nil {
			writeStoreErr(w, r, err)
			return
		}
	}
	data := make([]crmcontracts.Relationship, 0, len(rels))
	for _, rel := range rels {
		data = append(data, wireRelationship(rel))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.RelationshipListResponse{Data: data, Page: pageInfo(page)})
}

// SetProjectStakeholder attaches a person to a project with a role. It is
// a PUT because it is idempotent per person: re-stating an existing edge
// updates its role rather than raising a duplicate, which is what a caller
// correcting a role actually means.
func (h Handlers) SetProjectStakeholder(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.SetProjectStakeholderParams) {
	var req crmcontracts.SetProjectStakeholderRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	role := string(req.Role)
	rel, err := h.store.SetProjectStakeholder(r.Context(), SetProjectStakeholderInput{
		ProjectID: pathID[ids.ProjectKind](id),
		PersonID:  pathID[ids.PersonKind](req.PersonId),
		Role:      role,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireRelationship(rel))
}

// RemoveProjectStakeholder detaches a person from a project by archiving
// the edge — detaching is not deleting.
func (h Handlers) RemoveProjectStakeholder(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, personID openapi_types.UUID, _ crmcontracts.RemoveProjectStakeholderParams) {
	err := h.store.RemoveProjectStakeholder(r.Context(),
		pathID[ids.ProjectKind](id), ids.From[ids.PersonKind](ids.UUID(personID)))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetProjectCompany puts a company on a project with a role, or re-roles the
// edge that already exists, and answers the project's companies afterwards —
// the whole list rather than the one edge, because that is what a caller does
// next with the answer: render who is on this project.
func (h Handlers) SetProjectCompany(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.SetProjectCompanyParams) {
	var req crmcontracts.SetProjectCompanyRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	role := ""
	if req.Role != nil {
		role = *req.Role
	}
	on, err := h.store.SetProjectCompany(r.Context(), SetProjectCompanyInput{
		ProjectID:      pathID[ids.ProjectKind](id),
		OrganizationID: pathID[ids.OrganizationKind](req.OrganizationId),
		Role:           role,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ProjectCompanyListResponse{Data: wireProjectCompanies(on)})
}

// RemoveProjectCompany takes a company off a project by archiving the edge —
// taking off is not deleting, and the company keeps every record it owns.
func (h Handlers) RemoveProjectCompany(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, organizationID openapi_types.UUID, _ crmcontracts.RemoveProjectCompanyParams) {
	err := h.store.RemoveProjectCompany(r.Context(),
		pathID[ids.ProjectKind](id), ids.From[ids.OrganizationKind](ids.UUID(organizationID)))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// wireProjectCompanies maps the store's rows onto the contract's shape.
func wireProjectCompanies(on []ProjectCompany) []crmcontracts.ProjectCompany {
	out := make([]crmcontracts.ProjectCompany, 0, len(on))
	for _, one := range on {
		out = append(out, crmcontracts.ProjectCompany{
			OrganizationId: openapi_types.UUID(one.OrganizationID.UUID),
			DisplayName:    one.DisplayName,
			Role:           one.Role,
		})
	}
	return out
}
