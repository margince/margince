// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) capturedLeadPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	if !capturedLeadTargetPinned(approval) {
		return out, nil
	}
	captured, err := commercialRequest[capturedLead](approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(captured.FullName) == "" && captured.CompanyName == "" && captured.Title == "" {
		return out, nil
	}
	version := int64(*approval.TargetVersion)
	plan, err := contacts.NewStore(r.db).PreviewCapturedLeadFold(ctx, ids.From[ids.LeadKind](ids.UUID(*approval.TargetEntityId)), contacts.CapturedLeadFields{FullName: captured.FullName, Email: captured.Email, CompanyName: captured.CompanyName, Title: captured.Title}, &version)
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
	out.Changes, err = r.capturedLeadChanges(ctx, plan)
	if err != nil {
		return nil, err
	}
	target, err := r.referenceValue(ctx, "lead_id", approval.TargetEntityId.String())
	if previewUnavailable(err) {
		out.Changes = []crmcontracts.ApprovalPreviewChange{}
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details = &[]crmcontracts.ApprovalPreviewDetail{{Field: "capture_target", Value: target}, previewDetail("capture_full_name", captured.FullName), previewDetail("capture_company_name", captured.CompanyName), previewDetail("capture_title", captured.Title)}
	out.Effects, out.CurrentVersion, out.State = &[]string{"fill_empty_lead_fields"}, new(int(version)), previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) capturedLeadChanges(ctx context.Context, plan contacts.CapturedLeadFoldPreview) ([]crmcontracts.ApprovalPreviewChange, error) {
	current, err := json.Marshal(plan.Current)
	if err != nil {
		return nil, err
	}
	changes := map[string]json.RawMessage{}
	for _, field := range []struct {
		name  string
		value *string
	}{{fieldFullName, plan.Patch.FullName}, {companyNameField, plan.Patch.CompanyName}, {fieldTitle, plan.Patch.Title}} {
		if field.value == nil {
			continue
		}
		raw, err := json.Marshal(*field.value)
		if err != nil {
			return nil, err
		}
		changes[field.name] = raw
	}
	before, err := previewCurrentFields(current, changes)
	if err != nil {
		return nil, err
	}
	return r.changedValues(ctx, captureCollisionKind, before, changes)
}

func capturedLeadTargetPinned(approval crmcontracts.Approval) bool {
	return approval.TargetEntityType != nil && *approval.TargetEntityType == captureCollisionTarget && approval.TargetEntityId != nil && approval.TargetVersion != nil
}
