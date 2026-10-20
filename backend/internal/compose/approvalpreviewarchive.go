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
		if previewRouteMatches(args, "archive_record", expected.RecordType, expected.ID, "DELETE") {
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
	var effect string
	switch cmd.RecordType {
	case "relationship":
		effect = "archive_relationship_preserve_records"
	case "company":
		effect = "archive_company_detach_scope"
	case "contact":
		effect = "archive_contact_detach_scope"
	case "project":
		effect = "archive_project_detach_scope"
	case "deal":
		effect = "archive_deal_detach_scope"
	case "activity":
		effect = "archive_activity_preserve_content"
	case "product":
		effect = "archive_product_preserve_snapshots"
	case "offer_template":
		effect = "archive_template_preserve_offers"
	case "offer":
		effect = "archive_offer_preserve_lines"
	default:
		return out, nil
	}
	call := agents.NewArchiveCall(r.records, installationLanguage(r.db.Pool()), cmd)
	if err := call.Guards(ctx); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	if cmd.RecordType == "relationship" {
		return r.relationshipArchivePreview(ctx, approval, out)
	}
	details, err := r.archiveDetails(ctx, cmd.RecordType, fields)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if cmd.RecordType == "deal" || cmd.RecordType == "project" || cmd.RecordType == "contact" || cmd.RecordType == "company" {
		scope, err := r.recordArchiveDetails(ctx, approval)
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
		details = append(details, scope...)
	}
	if cmd.RecordType == "contact" || cmd.RecordType == "company" {
		children, err := r.archiveChildDetails(ctx, cmd.RecordType, fields)
		if previewUnavailable(err) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		details = append(details, children...)
	}
	out.Details = &details
	out.Effects, out.State = &[]string{effect}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) archiveDetails(ctx context.Context, recordType string, raw json.RawMessage) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	keys := []string{"name", "sku", "locale", "is_default"}
	name := "name"
	if recordType == "company" {
		name, keys = "display_name", []string{"display_name"}
	}
	if recordType == "contact" {
		name, keys = "full_name", []string{"full_name"}
	}
	if recordType == "deal" {
		keys = []string{"name", "company_id", "stage_id", "currency", "amount_minor"}
	}
	if recordType == "activity" {
		name, keys = "subject", []string{"subject"}
	}
	if recordType == "offer" {
		name = "offer_number"
		keys = []string{name, "revision", "deal_id", "status", "currency", "net_minor", "gross_minor"}
	}
	var label string
	if err := json.Unmarshal(fields[name], &label); err != nil || strings.TrimSpace(label) == "" {
		return nil, apperrors.ErrInvalidArgument
	}
	selected := map[string]any{}
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			if key == "status" {
				key = "offer_status"
			}
			selected[key] = value
		}
	}
	return r.creationFieldDetails(ctx, recordType, selected)
}
