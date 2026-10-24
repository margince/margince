// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The legal entities over HTTP. The logo upload and removal live in compose,
// beside the company logo's, because they share its decode and its storage.

import (
	"fmt"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListLegalEntities implements GET /legal-entities.
func (h Handlers) ListLegalEntities(w http.ResponseWriter, r *http.Request, params crmcontracts.ListLegalEntitiesParams) {
	includeArchived := params.IncludeArchived != nil && *params.IncludeArchived
	entities, err := h.store.ListLegalEntities(r.Context(), includeArchived)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := crmcontracts.LegalEntityListResponse{Data: make([]crmcontracts.LegalEntity, 0, len(entities))}
	for _, entity := range entities {
		out.Data = append(out.Data, LegalEntityWire(entity))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// CreateLegalEntity implements POST /legal-entities.
func (h Handlers) CreateLegalEntity(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.CreateLegalEntityRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	entity, err := h.store.CreateLegalEntity(r.Context(), LegalEntityInput{
		DisplayName: &body.DisplayName, LegalName: &body.LegalName, Country: &body.Country,
		Address: addressInput(body.Address), VatID: body.VatId, RegisterCourt: body.RegisterCourt,
		RegisterNumber: body.RegisterNumber, DefaultLanguage: body.DefaultLanguage,
		BankAccountHolder: body.BankAccountHolder, IBAN: body.Iban, BIC: body.Bic, Imprint: body.Imprint,
		Offering: offeringInput(body.Offering),
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/v1/legal-entities/%s", entity.ID))
	httperr.WriteJSON(w, http.StatusCreated, LegalEntityWire(entity))
}

// GetLegalEntity implements GET /legal-entities/{id}.
func (h Handlers) GetLegalEntity(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	entity, err := h.store.GetLegalEntity(r.Context(), pathID[ids.LegalEntityKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, LegalEntityWire(entity))
}

// UpdateLegalEntity implements PATCH /legal-entities/{id}.
func (h Handlers) UpdateLegalEntity(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateLegalEntityParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var body crmcontracts.UpdateLegalEntityRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	entity, err := h.store.UpdateLegalEntity(r.Context(), pathID[ids.LegalEntityKind](id), LegalEntityInput{
		DisplayName: body.DisplayName, LegalName: body.LegalName, Country: body.Country,
		Address: addressInput(body.Address), VatID: body.VatId, RegisterCourt: body.RegisterCourt,
		RegisterNumber: body.RegisterNumber, DefaultLanguage: body.DefaultLanguage,
		BankAccountHolder: body.BankAccountHolder, IBAN: body.Iban, BIC: body.Bic, Imprint: body.Imprint,
		Offering: offeringInput(body.Offering),
	}, ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, LegalEntityWire(entity))
}

// ArchiveLegalEntity implements POST /legal-entities/{id}/archive.
func (h Handlers) ArchiveLegalEntity(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ArchiveLegalEntityParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	entity, err := h.store.ArchiveLegalEntity(r.Context(), pathID[ids.LegalEntityKind](id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, LegalEntityWire(entity))
}

// GetLegalEntityLogo implements GET /legal-entities/{id}/logo.
func (h Handlers) GetLegalEntityLogo(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	key, err := h.store.LegalEntityLogoKey(r.Context(), pathID[ids.LegalEntityKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// No write-back: that path rewrites a COMPANY's stored mark, and an
	// uploaded entity logo is stored trimmed already.
	h.streamLogoKey(w, r, id, LogoWide, key, "GetLegalEntityLogo", logoCacheControl, false)
}

// ListLegalEntityMembers implements GET /legal-entities/{id}/members.
func (h Handlers) ListLegalEntityMembers(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	members, err := h.store.ListLegalEntityMembers(r.Context(), pathID[ids.LegalEntityKind](id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := crmcontracts.LegalEntityMemberListResponse{Data: make([]crmcontracts.LegalEntityMember, 0, len(members))}
	for _, m := range members {
		out.Data = append(out.Data, crmcontracts.LegalEntityMember{
			UserId: openapi_types.UUID(m.UserID.UUID), LegalEntityId: openapi_types.UUID(m.LegalEntityID.UUID),
			AssignedAt: m.AssignedAt,
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// AssignLegalEntityMember implements PUT /legal-entities/{id}/members.
func (h Handlers) AssignLegalEntityMember(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.AssignLegalEntityMemberRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	user := ids.From[ids.UserKind](ids.UUID(body.UserId))
	if err := h.store.AssignLegalEntityMember(r.Context(), pathID[ids.LegalEntityKind](id), user); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LegalEntityWire renders an entity for the contract. Exported because the
// logo upload in compose answers with the entity it changed.
func LegalEntityWire(e LegalEntity) crmcontracts.LegalEntity {
	a := e.Address
	return crmcontracts.LegalEntity{
		Id: openapi_types.UUID(e.ID.UUID), IsDefault: e.IsDefault, DisplayName: e.DisplayName, LegalName: e.LegalName,
		Address: crmcontracts.LegalEntityAddress{Line1: a.Line1, Line2: a.Line2, PostalCode: a.PostalCode, City: a.City, Region: a.Region},
		VatId:   e.VatID, RegisterCourt: e.RegisterCourt, RegisterNumber: e.RegisterNumber,
		Country: e.Country, DefaultLanguage: e.DefaultLanguage, BankAccountHolder: e.BankAccountHolder,
		Iban: e.IBAN, Bic: e.BIC, Imprint: e.Imprint, Offering: offeringWire(e.Offering), LogoUrl: legalEntityLogoURL(e.EntityIdentity),
		MemberCount: e.MemberCount, Version: e.Version, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
		ArchivedAt: e.ArchivedAt,
	}
}

// legalEntityLogoURL is the company's logo URL on the default entity, and the
// entity's own endpoint on every other. It carries LogoURL's digest token, so a
// replaced logo takes a new URL.
func legalEntityLogoURL(e EntityIdentity) *string {
	if e.IsDefault {
		if e.AnchorCompanyID.IsZero() {
			return nil
		}
		return LogoURL(e.AnchorCompanyID.UUID, e.LogoObjectKey, LogoWide)
	}
	if e.LogoObjectKey == nil || *e.LogoObjectKey == "" {
		return nil
	}
	path := fmt.Sprintf("/v1/legal-entities/%s/logo/image?v=%s", e.ID, logoRevisionDigest(*e.LogoObjectKey))
	return &path
}

func addressInput(a *crmcontracts.LegalEntityAddress) LegalEntityAddress {
	if a == nil {
		return LegalEntityAddress{}
	}
	return LegalEntityAddress{Line1: a.Line1, Line2: a.Line2, PostalCode: a.PostalCode, City: a.City, Region: a.Region}
}

func offeringInput(o *crmcontracts.LegalEntityOffering) LegalEntityOffering {
	if o == nil {
		return LegalEntityOffering{}
	}
	return LegalEntityOffering{OfferSummary: o.OfferSummary, ICP: o.Icp, ValueProposition: o.ValueProposition, USP: o.Usp}
}

func offeringWire(o LegalEntityOffering) *crmcontracts.LegalEntityOffering {
	return &crmcontracts.LegalEntityOffering{OfferSummary: o.OfferSummary, Icp: o.ICP, ValueProposition: o.ValueProposition, Usp: o.USP}
}
