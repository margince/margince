// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func archivePreviewCommand(approval crmcontracts.Approval) (agents.ArchiveCommand, error) {
	var empty agents.ArchiveCommand
	if approval.ProposedChange == nil || approval.TargetEntityId == nil || approval.TargetEntityType == nil {
		return empty, apperrors.ErrInvalidArgument
	}
	expected := agents.ArchiveCommand{RecordType: *approval.TargetEntityType, ID: ids.UUID(*approval.TargetEntityId)}
	args := *approval.ProposedChange
	if _, rest := args["operation"]; rest {
		if previewRouteMatches(args, toolArchiveRecord, expected.RecordType, expected.ID, "DELETE") {
			return expected, nil
		}
		return empty, apperrors.ErrInvalidArgument
	}
	request, err := commercialRequest[struct {
		RecordType string   `json:"record_type"`
		ID         ids.UUID `json:"id"`
	}](approval)
	if err != nil {
		return empty, err
	}
	if request.RecordType != expected.RecordType || request.ID != expected.ID {
		return empty, apperrors.ErrInvalidArgument
	}
	return expected, nil
}

func (r approvalPreviewReader) archivePreview(ctx context.Context, approval crmcontracts.Approval, fields json.RawMessage, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	cmd, err := archivePreviewCommand(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	effect := archiveEffect(cmd.RecordType)
	if effect == "" {
		return out, nil
	}
	call := agents.NewArchiveCall(r.records, installationLanguage(r.db.Pool()), cmd)
	if err := call.Guards(ctx); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	if cmd.RecordType == string(recordTypeRelationship) {
		return r.relationshipArchivePreview(ctx, approval, out)
	}
	details, err := r.archiveDetails(ctx, cmd.RecordType, fields)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	children, err := r.archiveRelatedDetails(ctx, approval, cmd.RecordType, fields)
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
	details = append(details, children...)
	out.Details = &details
	out.Effects, out.State = &[]string{effect}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) archiveDetails(ctx context.Context, recordType string, raw json.RawMessage) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	keys := []string{previewFieldName, "sku", previewFieldLocale, "is_default"}
	name := previewFieldName
	if recordType == string(recordTypeCompany) {
		name, keys = fieldDisplayName, []string{fieldDisplayName}
	}
	if recordType == string(recordTypeContact) {
		name, keys = fieldFullName, []string{fieldFullName}
	}
	if recordType == string(recordTypeDeal) {
		keys = []string{previewFieldName, refColCompany, fieldStageID, dealTargetCurrency, acceptFieldAmountMinor}
	}
	if recordType == string(recordTypeActivity) {
		name, keys = activityTargetSubject, []string{activityTargetSubject}
	}
	if recordType == string(recordTypeOffer) {
		name = "offer_number"
		keys = []string{name, "revision", dealIDField, previewFieldStatus, dealTargetCurrency, previewFieldNetMinor, previewFieldGrossMinor}
	}
	var label string
	if err := json.Unmarshal(fields[name], &label); err != nil || strings.TrimSpace(label) == "" {
		return nil, apperrors.ErrInvalidArgument
	}
	selected := map[string]any{}
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			if key == previewFieldStatus {
				key = "offer_status"
			}
			selected[key] = value
		}
	}
	return r.creationFieldDetails(ctx, recordType, selected)
}

func archiveEffect(recordType string) string {
	switch recordType {
	case string(recordTypeRelationship):
		return "archive_relationship_preserve_records"
	case string(recordTypeCompany):
		return "archive_company_detach_scope"
	case string(recordTypeContact):
		return "archive_contact_detach_scope"
	case string(recordTypeProject):
		return "archive_project_detach_scope"
	case string(recordTypeDeal):
		return "archive_deal_detach_scope"
	case string(recordTypeActivity):
		return "archive_activity_preserve_content"
	case string(recordTypeProduct):
		return "archive_product_preserve_snapshots"
	case string(recordTypeOfferTemplate):
		return "archive_template_preserve_offers"
	case string(recordTypeOffer):
		return "archive_offer_preserve_lines"
	default:
		return ""
	}
}

func (r approvalPreviewReader) archiveRelatedDetails(ctx context.Context, approval crmcontracts.Approval, recordType string, fields json.RawMessage) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var details []crmcontracts.ApprovalPreviewDetail
	switch recordType {
	case string(recordTypeDeal), string(recordTypeProject), string(recordTypeContact), string(recordTypeCompany):
		scope, err := r.recordArchiveDetails(ctx, approval)
		if err != nil {
			return nil, err
		}
		details = append(details, scope...)
	}
	if recordType == string(recordTypeContact) || recordType == string(recordTypeCompany) {
		children, err := r.archiveChildDetails(ctx, recordType, fields)
		if err != nil {
			return nil, err
		}
		details = append(details, children...)
	}
	return details, nil
}
