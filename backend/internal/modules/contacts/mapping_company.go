// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The company half of the contract request mappings in mapping.go.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// companyVisibilityArg is contactVisibilityArg for the other record. Two
// functions rather than one generic: the wire enums are distinct generated
// types, and the thing they share is that neither needs translating.
func companyVisibilityArg(v *crmcontracts.UpdateCompanyRequestVisibility) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

// CompanyCreationInput keeps request conversion shared by creation and its approval preview.
func CompanyCreationInput(req crmcontracts.CreateCompanyRequest) (CreateCompanyInput, error) {
	return companyCreateInputAdmitting(req, false)
}

// companyCreateInputFromImporter: see contactCreateInputFromImporter.
func companyCreateInputFromImporter(req crmcontracts.CreateCompanyRequest) (CreateCompanyInput, error) {
	return companyCreateInputAdmitting(req, true)
}

func companyCreateInputAdmitting(req crmcontracts.CreateCompanyRequest, importer bool) (CreateCompanyInput, error) {
	displayName, err := httperr.RequireNonBlank("display_name", req.DisplayName)
	if err != nil {
		return CreateCompanyInput{}, err
	}
	req.DisplayName = displayName
	source, err := httperr.RequireNonBlank(fieldSource, req.Source)
	if err != nil {
		return CreateCompanyInput{}, err
	}
	req.Source = source
	if err := provenance.RefuseWireAdmitting(req.Source, req.SourceSystem, importer); err != nil {
		return CreateCompanyInput{}, err
	}
	author, err := storekit.AdmitSourceAuthor(req.SourceAuthorId, req.SourceAuthorName, req.SourceSystem, importer)
	if err != nil {
		return CreateCompanyInput{}, err
	}
	if err := provenance.AdmitSourceID(req.SourceSystem, req.SourceId); err != nil {
		return CreateCompanyInput{}, err
	}
	in := CreateCompanyInput{
		DisplayName:     req.DisplayName,
		LegalName:       req.LegalName,
		LinkedInURL:     req.LinkedinUrl,
		Description:     req.Description,
		Industry:        req.Industry,
		Phone:           req.Phone,
		Email:           emailArg(req.Email),
		Source:          req.Source,
		SourceSystem:    req.SourceSystem,
		SourceID:        req.SourceId,
		Author:          author,
		OwnerID:         idArg[ids.UserKind](req.OwnerId),
		ParentCompanyID: idArg[ids.CompanyKind](req.ParentCompanyId),
		CustomFields:    req.AdditionalProperties,
	}
	in.Address = req.Address
	if req.SizeBand != nil {
		band := string(*req.SizeBand)
		in.SizeBand = &band
	}
	if req.Domains != nil {
		for _, d := range *req.Domains {
			in.Domains = append(in.Domains, CompanyDomainInput{
				Domain:    d.Domain,
				IsPrimary: d.IsPrimary != nil && *d.IsPrimary,
			})
		}
	}
	return in, nil
}

func companyUpdateInput(req crmcontracts.UpdateCompanyRequest, ifVersion *int64) UpdateCompanyInput {
	in := UpdateCompanyInput{
		DisplayName:           req.DisplayName,
		LegalName:             req.LegalName,
		Description:           req.Description,
		Industry:              req.Industry,
		OwnerID:               idArg[ids.UserKind](req.OwnerId),
		ParentCompanyID:       idArg[ids.CompanyKind](req.ParentCompanyId),
		AssessAgainstEntityID: idArg[ids.LegalEntityKind](req.AssessAgainstEntityId),
		IfVersion:             ifVersion,
		CustomFields:          req.AdditionalProperties,
		LinkedInURL:           req.LinkedinUrl,
		Phone:                 req.Phone,
		Email:                 emailArg(req.Email),
		Visibility:            companyVisibilityArg(req.Visibility),
	}
	in.Address = req.Address
	if req.SizeBand != nil {
		band := string(*req.SizeBand)
		in.SizeBand = &band
	}
	if req.Domains != nil {
		desired := make([]CompanyDomainInput, 0, len(*req.Domains))
		for _, d := range *req.Domains {
			desired = append(desired, CompanyDomainInput{
				Domain:    d.Domain,
				IsPrimary: d.IsPrimary != nil && *d.IsPrimary,
			})
		}
		in.Domains = &desired
	}
	if req.Lifecycle != nil {
		lifecycle := string(*req.Lifecycle)
		in.Lifecycle = &lifecycle
	}
	if req.RelationshipTypes != nil {
		desired := make([]string, 0, len(*req.RelationshipTypes))
		for _, t := range *req.RelationshipTypes {
			desired = append(desired, string(t))
		}
		in.RelationshipTypes = &desired
	}
	in.ExtensionRelationshipTypes = req.ExtensionRelationshipTypes
	return in
}
