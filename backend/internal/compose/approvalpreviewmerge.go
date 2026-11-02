// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type mergeSurvivorshipReader interface {
	PreviewMergeSurvivorship(context.Context, string, ids.UUID, ids.UUID, *int64, *int64) (contacts.MergeSurvivorship, error)
}

func (r approvalPreviewReader) recordMergePreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	request, err := recordMergeRequest(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	reader, ok := r.records.(mergeSurvivorshipReader)
	if !ok {
		return out, nil
	}
	targetVersion := int64(*approval.TargetVersion)
	scope, err := reader.PreviewMergeSurvivorship(ctx, request.RecordType, request.SourceID, request.TargetID, nil, &targetVersion)
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
	if err := r.populateMerge(ctx, request, scope, out); err != nil {
		if errors.Is(err, apperrors.ErrVersionSkew) {
			out.State = previewStaleState
			return out, nil
		}
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	out.State = previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) populateMerge(ctx context.Context, request recordMergePreviewRequest, scope contacts.MergeSurvivorship, out *crmcontracts.ApprovalPreview) error {
	r.touch(mergeScopeTouches(scope))
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, subject := range []struct {
		field string
		id    ids.UUID
	}{{"merge_source", request.SourceID}, {"merge_survivor", request.TargetID}} {
		value, err := r.referenceValue(ctx, request.RecordType+"_id", subject.id.String())
		if err != nil {
			return err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: subject.field, Value: value})
	}
	for _, project := range []func(context.Context, contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewDetail, error){r.mergeLinkDetails, r.mergeRelationshipDetails, r.mergeParticipantDetails, r.mergeConversationDetails, r.mergeConsentDetails, r.mergeGrantDetails} {
		carried, err := project(ctx, scope)
		if err != nil {
			return err
		}
		details = append(details, carried...)
	}
	details = append(details, mergeContentDetails(scope)...)
	if request.RecordType == string(recordTypeCompany) {
		company, err := r.companyMergeBusinessDetails(ctx, request.SourceID, request.TargetID)
		if err != nil {
			return err
		}
		evidence, err := mergeCompanyEvidenceDetails(scope)
		if err != nil {
			return err
		}
		details = append(details, company...)
		details = append(details, evidence...)
		assignments, err := r.companyMergeAssignmentDetails(ctx, request.SourceID, request.TargetID)
		if err != nil {
			return err
		}
		details = append(details, assignments...)
	}
	changes, err := r.mergeFieldChanges(ctx, request, scope)
	if err != nil {
		return err
	}
	out.Details, out.Changes, out.CurrentVersion = &details, changes, new(int(scope.TargetVersion))
	effects := []string{"merge_records_release_source"}
	if request.RecordType == string(recordTypeCompany) {
		effects = append(effects, "merge_company_private_readings", "merge_company_preserve_receipts", "merge_company_reconcile_lookups")
	}
	out.Effects = &effects
	return nil
}

func (r approvalPreviewReader) mergeFieldChanges(ctx context.Context, request recordMergePreviewRequest, scope contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewChange, error) {
	target, err := r.records.Read(ctx, datasource.EntityRef{Type: datasource.EntityType(request.RecordType), ID: request.TargetID})
	if err != nil {
		return nil, err
	}
	if target.Version != scope.TargetVersion {
		return nil, apperrors.ErrVersionSkew
	}
	changes := map[string]json.RawMessage{}
	for field, value := range scope.Fields {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		changes[field] = raw
	}
	current, err := previewCurrentFields(target.Fields, changes)
	if err != nil {
		return nil, err
	}
	return r.changedValues(ctx, "merge_records", current, changes)
}
