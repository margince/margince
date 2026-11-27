// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListDealStakeholders serves the deal-scoped stakeholder view over
// the relationship table this module owns; the deal itself must be
// visible (the endpoint-scope rule then re-applies per edge).
func (h Handlers) ListDealStakeholders(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	dealID := pathID[ids.DealKind](id)
	kind := dealStakeholderKind
	rels, page, err := h.store.ListRelationships(r.Context(), ListRelationshipsInput{
		Kind:   &kind,
		DealID: &dealID,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if len(rels) == 0 {
		// Distinguish "no stakeholders" from "no such deal" without
		// leaking: the deal read carries its own row scope.
		if err := h.store.EnsureDealVisible(r.Context(), dealID); err != nil {
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

func (h Handlers) ListRelationships(w http.ResponseWriter, r *http.Request, params crmcontracts.ListRelationshipsParams) {
	in := ListRelationshipsInput{
		Limit:           params.Limit,
		IncludeArchived: params.IncludeArchived != nil && *params.IncludeArchived,
	}
	if params.Kind != nil {
		kind := string(*params.Kind)
		in.Kind = &kind
	}
	in.ContactID = idArg[ids.ContactKind](params.ContactId)
	in.CompanyID = idArg[ids.CompanyKind](params.CompanyId)
	in.DealID = idArg[ids.DealKind](params.DealId)
	if params.Cursor != nil {
		in.Cursor = *params.Cursor
	}
	rels, page, err := h.store.ListRelationships(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	data := make([]crmcontracts.Relationship, 0, len(rels))
	for _, rel := range rels {
		data = append(data, wireRelationship(rel))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.RelationshipListResponse{Data: data, Page: pageInfo(page)})
}

func (h Handlers) CreateRelationship(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.CreateRelationshipRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := relationshipCreateInput(req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	rel, err := h.store.CreateRelationship(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, wireRelationship(rel))
}

func (h Handlers) UpdateRelationship(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateRelationshipParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateRelationshipRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := relationshipUpdateInput(req, ifVersion, httperr.ClearedFields(r))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	rel, err := h.store.UpdateRelationship(r.Context(), ids.UUID(id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireRelationship(rel))
}

// ArchiveRelationship retires one edge, honouring If-Match where the caller
// named a version.
func (h Handlers) ArchiveRelationship(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ArchiveRelationshipParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	rel, err := h.store.ArchiveRelationship(r.Context(), ids.UUID(id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireRelationship(rel))
}

func (h Handlers) UpsertPartner(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpsertPartnerParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpsertPartnerRequest
	if !httperr.DecodeClosed(w, r, &req, nil) {
		return
	}
	in := UpsertPartnerInput{CompanyID: pathID[ids.CompanyKind](id), IfVersion: ifVersion}
	if req.PartnerKinds != nil {
		kinds := make([]string, 0, len(*req.PartnerKinds))
		for _, kind := range *req.PartnerKinds {
			kinds = append(kinds, string(kind))
		}
		in.PartnerKinds = &kinds
	}
	if req.PartnerRole != nil {
		in.PartnerRole = string(*req.PartnerRole)
	}
	if req.CertStatus != nil {
		status := string(*req.CertStatus)
		in.CertStatus = &status
	}
	if req.MarginTier != nil {
		tier := string(*req.MarginTier)
		in.MarginTier = &tier
	}
	in.RelationshipStage = req.RelationshipStage
	in.NextStep = req.NextStep
	if req.NextStepDueAt != nil {
		in.NextStepDueAt = &req.NextStepDueAt.Time
	}
	in.ServedSegments = req.ServedSegments
	in.Cleared = httperr.ClearedFields(r)
	if req.GateMetrics != nil {
		if req.GateMetrics.CertifiedStaff != nil {
			in.CertifiedStaff = new(int16(*req.GateMetrics.CertifiedStaff)) // #nosec G115 -- DecodeClosed enforces the generated 0..32767 bound.
		}
		if req.GateMetrics.RetentionRate != nil {
			in.RetentionRate = new(int16(*req.GateMetrics.RetentionRate)) // #nosec G115 -- DecodeClosed enforces the generated 0..100 bound.
		}
	}
	partner, err := h.store.UpsertPartner(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, partner)
}

// GetPartner answers the partner row a company plays, if it plays one.
func (h Handlers) GetPartner(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.GetPartnerParams) {
	partner, err := h.store.GetPartner(r.Context(), pathID[ids.CompanyKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, partner)
}

func (h Handlers) ListPartners(w http.ResponseWriter, r *http.Request, params crmcontracts.ListPartnersParams) {
	in := ListPartnersInput{Limit: params.Limit, Sort: params.Sort, RelationshipStage: params.RelationshipStage}
	switch {
	case params.PartnerKind != nil:
		in.PartnerKind = new(string(*params.PartnerKind))
	case params.PartnerRole != nil:
		in.PartnerKind = new(string(*params.PartnerRole))
	}
	if params.StageMeaning != nil {
		in.StageMeaning = new(string(*params.StageMeaning))
	}
	if params.CertStatus != nil {
		status := string(*params.CertStatus)
		in.CertStatus = &status
	}
	if params.Cursor != nil {
		in.Cursor = *params.Cursor
	}
	partners, page, err := h.store.ListPartners(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"data": partners, "page": pageInfo(page)})
}

// EndPartnerProgram retires a company's partner extension and keeps the company.
func (h Handlers) EndPartnerProgram(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.EndPartnerProgramParams) {
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	if err := h.store.EndPartnerProgram(r.Context(), pathID[ids.CompanyKind](id), version); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
