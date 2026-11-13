// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// ContactCreationInput keeps request conversion shared by creation and its approval preview.
func ContactCreationInput(req crmcontracts.CreateContactRequest) (CreateContactInput, error) {
	return contactCreateInputAdmitting(req, false)
}

// contactCreateInputFromImporter is ContactCreationInput for a declared importer
// (auth.DeclaredImporter, asked by the handler): it may stamp the mirror:
// namespace. provider.go keeps the closed door.
func contactCreateInputFromImporter(req crmcontracts.CreateContactRequest) (CreateContactInput, error) {
	return contactCreateInputAdmitting(req, true)
}

func contactCreateInputAdmitting(req crmcontracts.CreateContactRequest, importer bool) (CreateContactInput, error) {
	fullName, err := httperr.RequireNonBlank(contactNameColumn, req.FullName)
	if err != nil {
		return CreateContactInput{}, err
	}
	req.FullName = fullName
	if err := checkTypedContactNames(req.FullName, req.FirstName, req.LastName); err != nil {
		return CreateContactInput{}, err
	}
	source, err := httperr.RequireNonBlank(fieldSource, req.Source)
	if err != nil {
		return CreateContactInput{}, err
	}
	req.Source = source
	if err := provenance.RefuseWireAdmitting(req.Source, req.SourceSystem, importer); err != nil {
		return CreateContactInput{}, err
	}
	author, err := storekit.AdmitSourceAuthor(req.SourceAuthorId, req.SourceAuthorName, req.SourceSystem, importer)
	if err != nil {
		return CreateContactInput{}, err
	}
	if err := provenance.AdmitSourceID(req.SourceSystem, req.SourceId); err != nil {
		return CreateContactInput{}, err
	}
	in := CreateContactInput{
		FullName:              req.FullName,
		FirstName:             req.FirstName,
		LastName:              req.LastName,
		Title:                 req.Title,
		Source:                req.Source,
		SourceSystem:          req.SourceSystem,
		SourceID:              req.SourceId,
		Author:                author,
		OwnerID:               idArg[ids.UserKind](req.OwnerId),
		CreatedFromActivityID: (*ids.UUID)(req.CreatedFromActivityId),
		// The body's extra top-level keys (custom-field values); the
		// store decides which land (active catalog columns only).
		CustomFields:   req.AdditionalProperties,
		ConvertLeadIDs: leadIDsArg(req.ConvertLeadIds),
	}
	if req.Social != nil {
		in.Social = *req.Social
	}
	if in.Social, err = socialWithLinkedInURL(in.Social, req.LinkedinUrl, false); err != nil {
		return CreateContactInput{}, err
	}
	in.Address = req.Address
	in.Emails = contactEmailInputs(req.Emails)
	in.Phones = contactPhoneInputs(req.Phones)
	in.Employer, err = employerChoiceFromRequest(idArg[ids.CompanyKind](req.CompanyId), req.CompanyName, req.Source)
	return in, err
}
