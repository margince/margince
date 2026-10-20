// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"maps"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
)

func dealCreationRequest(approval crmcontracts.Approval) (deals.CreateDealInput, error) {
	req, tool, err := creationRequest[crmcontracts.CreateDealRequest](approval, "deal")
	if err != nil {
		return deals.CreateDealInput{}, err
	}
	if tool {
		req.Source = agents.ToolSource
	}
	return deals.DealCreationInput(req)
}

func (r approvalPreviewReader) dealCreationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	in, err := dealCreationRequest(approval)
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
	store := deals.NewStore(r.db, DealsInstallation()).WithFieldCatalog(customfields.NewService(r.db.Pool(), nil))
	plan, err := store.PreviewDealCreationForActor(ctx, in, creator)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, err := r.dealCreationDetails(ctx, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details, out.Effects, out.State = &details, &[]string{"create_deal_record"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) dealCreationDetails(ctx context.Context, in deals.CreateDealInput) ([]crmcontracts.ApprovalPreviewDetail, error) {
	fields := map[string]any{
		"name": in.Name, "amount_minor": in.AmountMinor, "expected_arr_minor": in.ExpectedArrMinor, "currency": in.Currency,
		"pipeline_id": in.PipelineID, "stage_id": in.StageID, "company_id": in.CompanyID,
		"partner_company_id": in.PartnerCompanyID, "partner_attribution": in.PartnerAttribution,
		"project_id": in.ProjectID, "owner_id": in.OwnerID, "expected_close_date": in.ExpectedClose,
		"source": in.Source, "source_system": in.SourceSystem,
		"source_author_id": in.Author.AuthorID, "source_author_name": in.Author.AuthorName,
		"description": in.Description, "commercial_motion": in.CommercialMotion,
		"priority": in.Priority, "acquisition_source": in.AcquisitionSource,
	}
	maps.Copy(fields, in.CustomFields)
	return r.creationFieldDetails(ctx, "deal", fields)
}
