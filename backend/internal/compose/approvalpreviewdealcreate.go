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
		previewFieldName: in.Name, acceptFieldAmountMinor: in.AmountMinor, previewFieldExpectedArrMinor: in.ExpectedArrMinor, dealTargetCurrency: in.Currency,
		fieldPipelineID: in.PipelineID, fieldStageID: in.StageID, refColCompany: in.CompanyID,
		refColPartnerCompany: in.PartnerCompanyID, previewFieldPartnerAttribution: in.PartnerAttribution,
		refColProject: in.ProjectID, bulkParamOwner: in.OwnerID, acceptFieldExpectedClose: in.ExpectedClose,
		fieldSource: in.Source, previewFieldSourceSystem: in.SourceSystem,
		previewFieldSourceAuthorID: in.Author.AuthorID, previewFieldSourceAuthorName: in.Author.AuthorName,
		descriptionField: in.Description, previewFieldCommercialMotion: in.CommercialMotion,
		previewFieldPriority: in.Priority, "acquisition_source": in.AcquisitionSource,
	}
	maps.Copy(fields, in.CustomFields)
	return r.creationFieldDetails(ctx, "deal", fields)
}
