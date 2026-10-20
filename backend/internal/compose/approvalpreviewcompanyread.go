// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func directCompanyRead(approval crmcontracts.Approval) bool {
	if approval.Kind == deepReadProposalKind {
		return true
	}
	if approval.Kind != enrichProposalKind || approval.ProposedChange == nil {
		return false
	}
	_, fields := (*approval.ProposedChange)["fields"]
	_, target := (*approval.ProposedChange)["company_id"]
	return fields && target
}

func (r approvalPreviewReader) companyReadPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return nil, err
	}
	proposal, err := contacts.UnmarshalDeepRead(raw)
	if err != nil || proposal.CompanyID.IsZero() {
		return out, nil
	}
	if approval.TargetEntityType == nil || *approval.TargetEntityType != "company" || approval.TargetEntityId == nil || ids.UUID(*approval.TargetEntityId) != proposal.CompanyID.UUID {
		return out, nil
	}
	plan, err := contacts.NewStore(r.db).PreviewCompanyRead(ctx, proposal)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if plan.Company.Version == nil {
		return out, nil
	}
	if approval.TargetVersion != nil && *approval.TargetVersion != int(*plan.Company.Version) {
		out.State = previewStaleState
		return out, nil
	}
	target, err := r.referenceValue(ctx, "company_id", proposal.CompanyID.String())
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{{Field: "company_id", Value: target}, previewLinkDetail("read_source_url", proposal.SourceURL)}
	for _, column := range plan.Columns {
		out.Changes = append(out.Changes, crmcontracts.ApprovalPreviewChange{Field: column.Field, Before: readPreviewText(column.Before), After: readPreviewText(column.After)})
	}
	details = append(details, companyReadEvidenceDetails(plan.Profiles, "read_profile_field")...)
	details = append(details, companyReadEvidenceDetails(plan.Facts, "read_fact_field")...)
	out.Details, out.State, out.CurrentVersion = &details, previewReadyState, new(int(*plan.Company.Version))
	out.Effects = &[]string{"apply_company_read"}
	return out, nil
}

func companyReadEvidenceDetails(values []contacts.CompanyReadValue, fieldKey string) []crmcontracts.ApprovalPreviewDetail {
	details := make([]crmcontracts.ApprovalPreviewDetail, 0, len(values)*7)
	for _, value := range values {
		details = append(details, previewDetail(fieldKey, value.Field), previewDetail("read_value_before", value.Before), previewDetail("read_value_after", value.After), previewDetail("read_value_proposed", value.Proposed), previewLinkDetail("read_source_url", value.SourceURL), previewDetail("read_source_quote", value.Quote), mergeBooleanDetail("read_value_retained", value.Retained))
	}
	return details
}

func readPreviewText(value string) crmcontracts.ApprovalPreviewValue {
	if value == "" {
		return crmcontracts.ApprovalPreviewValue{Kind: previewValueEmpty, Text: ""}
	}
	return crmcontracts.ApprovalPreviewValue{Kind: previewValueText, Text: value}
}
