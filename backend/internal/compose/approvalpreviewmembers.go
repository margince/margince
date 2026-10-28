// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) memberMergePreview(ctx context.Context, approval crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
	out := &crmcontracts.ApprovalPreview{Version: 1, State: previewIncompleteState, Changes: []crmcontracts.ApprovalPreviewChange{}}
	request, err := commercialRequest[memberMergePreviewRequest](approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if !request.matches(approval) || r.db == nil {
		return out, nil
	}
	preview, err := newMemberManager(r.db).Preview(ctx, request.SourceID, &request.TargetID, true)
	if errors.Is(err, apperrors.ErrConflict) {
		conflicts := []crmcontracts.ApprovalPreviewConflict{{Kind: "member_retirement_state_changed", Summary: "The member changed. Refresh the retirement preview.", BlocksApproval: true}}
		out.Conflicts = &conflicts
		return out, nil
	}
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if preview.Token != request.Token {
		out.State = previewStaleState
		return out, nil
	}
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("member_source", preview.SourceName+" ("+string(preview.SourceEmail)+")")}
	if preview.SurvivorName != nil && preview.SurvivorEmail != nil {
		details = append(details, previewDetail("member_target", *preview.SurvivorName+" ("+string(*preview.SurvivorEmail)+")"))
	}
	effects := []string{}
	for _, consequence := range preview.Consequences {
		effects = append(effects, "member_"+string(consequence))
	}
	out.Effects = &effects
	conflicts := []crmcontracts.ApprovalPreviewConflict{}
	for _, blocker := range preview.Blockers {
		conflicts = append(conflicts, crmcontracts.ApprovalPreviewConflict{Kind: "member_retirement_" + string(blocker), Summary: "Resolve the member retirement dependency.", BlocksApproval: true})
	}
	out.Details, out.Conflicts, out.State = &details, &conflicts, previewReadyState
	return out, nil
}

type memberMergePreviewRequest struct {
	SourceID ids.UUID `json:"source_id"`
	TargetID ids.UUID `json:"to_user_id"`
	Token    string   `json:"preview_token"`
}

func (request memberMergePreviewRequest) matches(approval crmcontracts.Approval) bool {
	if request.SourceID.IsZero() || request.TargetID.IsZero() || request.Token == "" {
		return false
	}
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil {
		return false
	}
	return *approval.TargetEntityType == string(recordTypeUserAdmin) && ids.UUID(*approval.TargetEntityId) == request.SourceID
}
