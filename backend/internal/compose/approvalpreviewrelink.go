// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type relinkPreviewRequest struct {
	ActivityID  ids.UUID   `json:"activity_id"`
	ActivityIDs []ids.UUID `json:"activity_ids"`
	EntityType  string     `json:"entity_type"`
	EntityID    ids.UUID   `json:"entity_id"`
	Replace     bool       `json:"replace_existing_of_type"`
}

func relinkApprovalInput(approval crmcontracts.Approval) ([]ids.UUID, activities.RelinkActivityInput, error) {
	var in activities.RelinkActivityInput
	req, err := commercialRequest[relinkPreviewRequest](approval)
	if err != nil {
		return nil, in, err
	}
	if approval.ProposedChange == nil || approval.TargetEntityId == nil || approval.TargetEntityType == nil || approval.TargetVersion == nil {
		return nil, in, apperrors.ErrInvalidArgument
	}
	in = activities.RelinkActivityInput{EntityType: req.EntityType, EntityID: req.EntityID, ReplaceExistingOfType: req.Replace}
	if approval.Kind == "relink_activity" {
		if *approval.TargetEntityType != "activity" {
			return nil, in, apperrors.ErrInvalidArgument
		}
		id := ids.UUID(*approval.TargetEntityId)
		_, rest := (*approval.ProposedChange)["operation"]
		if !rest && req.ActivityID != id {
			return nil, in, apperrors.ErrInvalidArgument
		}
		in.IfVersion = new(int64(*approval.TargetVersion))
		return []ids.UUID{id}, in, nil
	}
	if approval.Kind != "relink_activities" || req.EntityType != *approval.TargetEntityType || req.EntityID != ids.UUID(*approval.TargetEntityId) {
		return nil, in, apperrors.ErrInvalidArgument
	}
	return req.ActivityIDs, in, nil
}

func (r approvalPreviewReader) relinkPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	activityIDs, in, err := relinkApprovalInput(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	plans, err := r.activities.PreviewRelink(ctx, activityIDs, in)
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
	details, err := r.relinkDetails(ctx, in, plans)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	effects := []string{"relink_preserve_other_links"}
	for _, plan := range plans {
		if plan.StampsCorrespondence {
			effects = append(effects, "relink_project_retention")
			break
		}
	}
	out.Details, out.Effects, out.State = &details, &effects, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) relinkDetails(ctx context.Context, in activities.RelinkActivityInput, plans []activities.ActivityRelinkPreview) ([]crmcontracts.ApprovalPreviewDetail, error) {
	target, err := r.referenceValue(ctx, in.EntityType+"_id", in.EntityID.String())
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{{Field: "relink_destination", Value: target}}
	for _, plan := range plans {
		activity, err := r.referenceValue(ctx, "activity_id", plan.Activity.Id.String())
		if err != nil {
			return nil, err
		}
		field := "relink_unchanged_activity"
		if plan.AddsLink {
			field = "relink_activity"
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: field, Value: activity})
		for _, removed := range plan.Removed {
			value, err := r.referenceValue(ctx, in.EntityType+"_id", removed.String())
			if err != nil {
				return nil, err
			}
			details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "relink_removed_link", Value: value})
		}
		participants, err := r.relinkParticipantDetails(ctx, plan.Participants)
		if err != nil {
			return nil, err
		}
		details = append(details, participants...)
	}
	return details, nil
}

func (r approvalPreviewReader) relinkParticipantDetails(ctx context.Context, participants []activities.RelinkParticipantChange) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var details []crmcontracts.ApprovalPreviewDetail
	for _, participant := range participants {
		contact, err := r.referenceValue(ctx, "contact_id", participant.ContactID.String())
		if err != nil {
			return nil, err
		}
		field := "relink_participant_merge"
		if participant.Move {
			field = "relink_participant_move"
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: field, Value: contact}, previewDetail("relink_participant_role", participant.Role))
		if participant.Address != nil {
			details = append(details, previewDetail("relink_participant_address", *participant.Address))
		}
		if participant.ChannelUserID != nil {
			details = append(details, previewDetail("channel_account", *participant.ChannelUserID))
		}
		if participant.UserID != nil {
			user, err := r.referenceValue(ctx, "user_id", participant.UserID.String())
			if err != nil {
				return nil, err
			}
			details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "relink_participant_user", Value: user})
		}
	}
	return details, nil
}
