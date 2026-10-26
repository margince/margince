// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"maps"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func companyCreationRequest(approval crmcontracts.Approval) (contacts.CreateCompanyInput, error) {
	req, tool, err := creationRequest[crmcontracts.CreateCompanyRequest](approval, string(recordTypeCompany))
	if err != nil {
		return contacts.CreateCompanyInput{}, err
	}
	if tool {
		req.Source = agents.ToolSource
	}
	return contacts.CompanyCreationInput(req)
}

func (r approvalPreviewReader) companyCreationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	in, err := companyCreationRequest(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return r.creationPreviewDetails(ctx, approval, out, func(creator principal.Principal) ([]crmcontracts.ApprovalPreviewDetail, []string, error) {
		store := contacts.NewStore(r.db).WithFieldCatalog(customfields.NewService(r.db.Pool(), nil))
		plan, err := store.PreviewCompanyCreationForActor(ctx, in, creator)
		if err != nil {
			return nil, nil, err
		}
		return r.companyCreationDetails(ctx, plan)
	})
}

func (r approvalPreviewReader) companyCreationDetails(ctx context.Context, plan contacts.CompanyCreationPreview) ([]crmcontracts.ApprovalPreviewDetail, []string, error) {
	in := plan.Input
	industry, description := contacts.CompanyIndustryColumns(in.Industry, in.Description)
	fields := map[string]any{
		"display_name": in.DisplayName, fieldLegalName: in.LegalName, descriptionField: description,
		fieldIndustry: industry, fieldSizeBand: in.SizeBand, bulkParamOwner: in.OwnerID, refColParentCompany: in.ParentCompanyID,
		fieldLinkedIn: in.LinkedInURL, fieldSource: in.Source, previewFieldSourceSystem: in.SourceSystem,
		previewFieldSourceAuthorName: in.Author.AuthorName, previewFieldSourceAuthorID: in.Author.AuthorID,
		previewLifecycle: previewLifecycleUnknown,
	}
	maps.Copy(fields, plan.CustomFields)
	if err := creationFieldsVisible(ctx, string(recordTypeCompany), map[string]any{previewFieldDomains: in.Domains, addressField: in.Address}); err != nil {
		return nil, nil, err
	}
	maps.Copy(fields, creationAddressFields(in.Address))
	details, err := r.creationFieldDetails(ctx, string(recordTypeCompany), fields)
	if err != nil {
		return nil, nil, err
	}
	for _, domain := range in.Domains {
		field := "domain"
		if domain.IsPrimary {
			field = "creation_primary_domain"
		}
		details = append(details, previewDetail(field, domain.Domain))
	}
	effects := []string{"create_company_record", "create_workspace_visible"}
	if plan.AddressLookup {
		effects = append(effects, "geocode_if_enabled")
	}
	if plan.ReviewCompanyID != nil {
		value, err := r.referenceValue(ctx, refColCompany, plan.ReviewCompanyID.String())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: previewCreationDuplicate, Value: value})
		effects = append(effects, "queue_duplicate_review")
	}
	return details, effects, nil
}
