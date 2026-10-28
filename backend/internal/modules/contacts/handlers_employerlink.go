// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithConsumerMail is Store.WithConsumerMail for the store behind these handlers.
func (h Handlers) WithConsumerMail(read ConsumerMailReader) Handlers {
	h.store = h.store.WithConsumerMail(read)
	return h
}

// LinkContactEmployer serves POST /contacts/{id}/employer.
func (h Handlers) LinkContactEmployer(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.LinkContactEmployerParams) {
	var req crmcontracts.LinkContactEmployerRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	linked, err := h.store.LinkContactEmployer(r.Context(), pathID[ids.ContactKind](id), LinkEmployerInput{
		CompanyID:   idArg[ids.CompanyKind](req.CompanyId),
		CompanyName: req.CompanyName,
		Title:       req.Title,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, crmcontracts.LinkContactEmployerResult{
		Contact:        linked.Contact,
		CompanyId:      openapi_types.UUID(linked.CompanyID.UUID),
		CompanyCreated: linked.CompanyCreated,
		CompanyDomain:  linked.CompanyDomain,
		RelationshipId: openapi_types.UUID(linked.RelationshipID),
	})
}

// ListCompanyLinkCandidates serves GET /companies/{id}/link-candidates.
func (h Handlers) ListCompanyLinkCandidates(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	candidates, truncated, err := h.store.ListLinkCandidates(r.Context(), pathID[ids.CompanyKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	data := make([]crmcontracts.CompanyLinkCandidate, 0, len(candidates))
	for _, c := range candidates {
		data = append(data, crmcontracts.CompanyLinkCandidate{
			ContactId: openapi_types.UUID(c.ContactID.UUID),
			FullName:  c.FullName,
			Title:     c.Title,
			Email:     c.Email,
			Employer:  c.Employer,
		})
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.CompanyLinkCandidateList{Data: data, Truncated: truncated})
}

// LinkCompanyContacts serves POST /companies/{id}/link-contacts.
func (h Handlers) LinkCompanyContacts(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.LinkCompanyContactsParams) {
	var req crmcontracts.LinkCompanyContactsRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	contactIDs := make([]ids.UUID, 0, len(req.ContactIds))
	for _, contactID := range req.ContactIds {
		contactIDs = append(contactIDs, ids.UUID(contactID))
	}
	edges, err := h.store.LinkCompanyContacts(r.Context(), pathID[ids.CompanyKind](id), contactIDs)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	data := make([]crmcontracts.Relationship, 0, len(edges))
	for _, edge := range edges {
		data = append(data, wireRelationship(edge))
	}
	httperr.WriteJSON(w, http.StatusCreated, crmcontracts.LinkCompanyContactsResult{Data: data})
}
