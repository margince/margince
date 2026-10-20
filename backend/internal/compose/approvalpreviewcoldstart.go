// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

func (r approvalPreviewReader) coldStartPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return nil, err
	}
	source, fields, err := contacts.UnmarshalColdStartFields(raw)
	if err != nil || source == "" || len(fields) == 0 {
		return out, nil
	}
	plan, err := contacts.NewStore(r.db).PreviewColdStart(ctx, contacts.ApplyColdStartProfileInput{SourceURL: source, Fields: fields})
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{previewLinkDetail("read_source_url", source)}
	effects := []string{"apply_company_read"}
	if plan.QueuesVATCheck {
		effects = append(effects, "queue_vat_check")
	}
	if plan.Creates {
		details = append(details, previewDetail("display_name", plan.Name), previewDetail("creation_primary_domain", plan.Domain))
		effects = append(effects, "create_company_record")
	} else {
		target, err := r.referenceValue(ctx, "company_id", plan.Read.Company.Id.String())
		if previewUnavailable(err) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "company_id", Value: target})
		if plan.Read.Company.Version == nil {
			return out, nil
		}
		out.CurrentVersion = new(int(*plan.Read.Company.Version))
	}
	if plan.ReviewCompanyID != nil {
		duplicate, err := r.referenceValue(ctx, "company_id", plan.ReviewCompanyID.String())
		if previewUnavailable(err) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "creation_duplicate", Value: duplicate})
		effects = append(effects, "queue_duplicate_review")
	}
	for _, column := range plan.Read.Columns {
		out.Changes = append(out.Changes, crmcontracts.ApprovalPreviewChange{Field: column.Field, Before: readPreviewText(column.Before), After: readPreviewText(column.After)})
	}
	details = append(details, companyReadEvidenceDetails(plan.Read.Profiles, "read_profile_field")...)
	out.Details, out.Effects, out.State = &details, &effects, previewReadyState
	return out, nil
}
