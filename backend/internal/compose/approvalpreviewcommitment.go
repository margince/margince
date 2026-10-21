// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (r approvalPreviewReader) commitmentPreview(ctx context.Context, approval crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
	out := &crmcontracts.ApprovalPreview{Version: 1, State: previewIncompleteState, Changes: []crmcontracts.ApprovalPreviewChange{}, ReviewerReason: previewReviewer(ctx, approval)}
	out.Evidence = &[]crmcontracts.ApprovalEvidence{}
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return nil, err
	}
	var proposal CommitmentTaskProposal
	if err := json.Unmarshal(raw, &proposal); err != nil {
		return out, nil //nolint:nilerr // A malformed stored proposal is shown as incomplete.
	}
	if !commitmentPreviewIdentityMatches(approval, proposal) {
		return out, nil
	}
	sourceType := crmcontracts.ApprovalEvidenceSourceType(string(recordTypeActivity))
	citation, err := r.currentCitation(ctx, crmcontracts.ApprovalEvidence{SourceType: &sourceType, SourceId: new(openapi_types.UUID(proposal.SourceActivityID)), EvidenceSnippet: proposal.Quote})
	if state := evidenceFailureState(err); state != "" {
		out.State = state
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	recipients, err := r.commitmentRecipientNames(ctx, proposal)
	if state := evidenceFailureState(err); state != "" {
		out.State = state
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if proposal.ExistingTaskID != nil {
		out, err = r.commitmentRecipientAttachment(ctx, proposal, out, recipients)
		if err != nil {
			return nil, err
		}
		if out.State == previewReadyState {
			out.Evidence = &[]crmcontracts.ApprovalEvidence{citation}
		}
		return out, nil
	}
	return r.commitmentCreationPreview(ctx, proposal, out, recipients, citation)
}

func (r approvalPreviewReader) commitmentCreationPreview(ctx context.Context, proposal CommitmentTaskProposal, out *crmcontracts.ApprovalPreview, recipients []string, citation crmcontracts.ApprovalEvidence) (*crmcontracts.ApprovalPreview, error) {
	changes := []crmcontracts.ApprovalPreviewChange{commitmentCreateValue("summary", previewValueText, proposal.Summary)}
	actor, found := principal.Actor(ctx)
	if !found {
		return out, nil
	}
	assignee := actor.UserID
	if proposal.SeatID != nil {
		assignee = *proposal.SeatID
	}
	owner, err := r.referenceValue(ctx, bulkParamOwner, assignee.String())
	if previewUnavailable(err) {
		out.State = previewUnavailableState
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	changes = append(changes, crmcontracts.ApprovalPreviewChange{Field: bulkParamOwner, Before: crmcontracts.ApprovalPreviewValue{Kind: previewValueEmpty}, After: owner})

	if proposal.DueDate != "" {
		changes = append(changes, commitmentCreateValue("due_date", previewValueDate, proposal.DueDate))
	}
	if len(recipients) > 0 {
		changes = append(changes, commitmentCreateValue("recipients", previewValueText, strings.Join(recipients, ", ")))
	}
	out.Changes, out.Evidence, out.State = changes, &[]crmcontracts.ApprovalEvidence{citation}, previewReadyState
	return out, nil
}

func commitmentCreateValue(field, kind, text string) crmcontracts.ApprovalPreviewChange {
	return crmcontracts.ApprovalPreviewChange{Field: field, Before: crmcontracts.ApprovalPreviewValue{Kind: previewValueEmpty}, After: crmcontracts.ApprovalPreviewValue{Kind: kind, Text: approvals.SanitizeSummary(text)}}
}

func (r approvalPreviewReader) commitmentRecipientNames(ctx context.Context, proposal CommitmentTaskProposal) ([]string, error) {
	named := commitmentClaimIDs(proposal.ClaimID, proposal.ClaimIDs)
	claims, err := contacts.NewStore(r.db).ClaimsByIDs(ctx, named)
	if err != nil {
		return nil, err
	}
	if len(claims) != len(named) {
		return nil, apperrors.ErrNotFound
	}
	names := make([]string, 0, len(claims))
	open := len(claims) == 0
	for _, claim := range claims {
		if claim.Source != proposal.SourceActivityID || claim.Locator != proposal.Locator {
			return nil, apperrors.ErrVersionSkew
		}
		names = append(names, claim.DisplayName)
		open = open || claim.Status == commitmentStateOpen
	}
	if !open {
		return nil, apperrors.ErrVersionSkew
	}
	return names, nil
}

func (r approvalPreviewReader) commitmentRecipientAttachment(ctx context.Context, proposal CommitmentTaskProposal, out *crmcontracts.ApprovalPreview, recipients []string) (*crmcontracts.ApprovalPreview, error) {
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := commitmentRecipientTarget(ctx, tx, r.activities, proposal)
		return err
	})
	if state := evidenceFailureState(err); state != "" {
		out.State = state
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return out, nil
	}
	task, err := r.referenceValue(ctx, "activity_id", proposal.ExistingTaskID.String())
	if state := evidenceFailureState(err); state != "" {
		out.State = state
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Changes = []crmcontracts.ApprovalPreviewChange{
		{Field: "existing_task_id", Before: task, After: task},
		commitmentCreateValue("recipients", previewValueText, strings.Join(recipients, ", ")),
	}
	out.Effects = &[]string{"attach_task_recipients"}
	out.State = previewReadyState
	return out, nil
}

func commitmentPreviewIdentityMatches(approval crmcontracts.Approval, proposal CommitmentTaskProposal) bool {
	return !proposal.SourceActivityID.IsZero() && proposal.Locator != "" && strings.TrimSpace(proposal.Summary) != "" && proposal.Quote != "" &&
		approval.TargetEntityType != nil && *approval.TargetEntityType == string(recordTypeActivity) &&
		approval.TargetEntityId != nil && ids.UUID(*approval.TargetEntityId) == proposal.SourceActivityID
}
