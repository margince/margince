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
)

func companyCreationRequest(approval crmcontracts.Approval) (contacts.CreateCompanyInput, error) {
	req, tool, err := creationRequest[crmcontracts.CreateCompanyRequest](approval, "company")
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
	creator, err := r.creationAuthority(ctx, approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	store := contacts.NewStore(r.db).WithFieldCatalog(customfields.NewService(r.db.Pool(), nil))
	plan, err := store.PreviewCompanyCreationForActor(ctx, in, creator)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, effects, err := r.companyCreationDetails(ctx, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details, out.Effects, out.State = &details, &effects, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) companyCreationDetails(ctx context.Context, plan contacts.CompanyCreationPreview) ([]crmcontracts.ApprovalPreviewDetail, []string, error) {
	in := plan.Input
	fields := map[string]any{
		"display_name": in.DisplayName, "legal_name": in.LegalName, "description": in.Description,
		"industry": in.Industry, "size_band": in.SizeBand, "owner_id": in.OwnerID, "parent_company_id": in.ParentCompanyID,
		"linkedin_url": in.LinkedInURL, "source": in.Source, "source_system": in.SourceSystem,
		"source_author_name": in.Author.AuthorName, "source_author_id": in.Author.AuthorID,
		"lifecycle": "unknown",
	}
	maps.Copy(fields, plan.CustomFields)
	if err := creationFieldsVisible(ctx, "company", map[string]any{"domains": in.Domains, "address": in.Address}); err != nil {
		return nil, nil, err
	}
	maps.Copy(fields, creationAddressFields(in.Address))
	details, err := r.creationFieldDetails(ctx, "company", fields)
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
		value, err := r.referenceValue(ctx, "company_id", plan.ReviewCompanyID.String())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "creation_duplicate", Value: value})
		effects = append(effects, "queue_duplicate_review")
	}
	return details, effects, nil
}
