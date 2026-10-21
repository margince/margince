// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) relationshipArchivePreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	scope, err := contacts.NewStore(r.db).PreviewRelationshipArchive(ctx, ids.UUID(*approval.TargetEntityId), new(int64(*approval.TargetVersion)))
	if errors.Is(err, apperrors.ErrVersionSkew) {
		out.State = previewStaleState
		return out, nil
	}
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, err := r.relationshipArchiveDetails(ctx, scope.Relationship)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	effects := []string{"archive_relationship_preserve_records"}
	if scope.Successor != nil {
		if scope.Successor.CompanyId == nil {
			return out, nil
		}
		company, err := r.referenceValue(ctx, refColCompany, scope.Successor.CompanyId.String())
		if previewUnavailable(err) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "archive_primary_employer", Value: company})
		effects = append(effects, "archive_relationship_promote_employer")
	} else if scope.Relationship.Kind == "employment" && scope.Relationship.IsCurrentPrimary != nil && *scope.Relationship.IsCurrentPrimary {
		effects = append(effects, "archive_relationship_no_primary")
	}
	out.Details, out.Effects, out.State = &details, &effects, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) relationshipArchiveDetails(ctx context.Context, edge crmcontracts.Relationship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("relationship_kind", string(edge.Kind))}
	for _, endpoint := range []struct {
		field, kind string
		id          *crmcontracts.Id
	}{
		{companionContactField, string(recordTypeContact), edge.ContactId},
		{refColCompany, string(recordTypeCompany), edge.CompanyId},
		{dealIDField, string(recordTypeDeal), edge.DealId},
		{"project_id", string(recordTypeProject), edge.ProjectId},
		{"counterparty_contact_id", string(recordTypeContact), edge.CounterpartyContactId},
		{"counterparty_company_id", string(recordTypeCompany), edge.CounterpartyCompanyId},
	} {
		if endpoint.id == nil {
			continue
		}
		value, err := r.referenceValue(ctx, endpoint.kind+"_id", endpoint.id.String())
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: endpoint.field, Value: value})
	}
	if edge.Role != nil {
		details = append(details, previewDetail("archive_role", *edge.Role))
	}
	if edge.StartedAt != nil {
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "relationship_started", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueDate, Text: edge.StartedAt.String()}})
	}
	if edge.EndedAt != nil {
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "relationship_ended", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueDate, Text: edge.EndedAt.String()}})
	}
	if edge.StartedPrecision != nil {
		details = append(details, previewDetail("relationship_start_precision", string(*edge.StartedPrecision)))
	}
	if edge.EndedPrecision != nil {
		details = append(details, previewDetail("relationship_end_precision", string(*edge.EndedPrecision)))
	}
	return details, nil
}
