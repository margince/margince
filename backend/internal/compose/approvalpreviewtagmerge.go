// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type tagMergePreviewRequest struct {
	SourceID      ids.UUID `json:"tag_id"`
	TargetID      ids.UUID `json:"into_tag_id"`
	TargetVersion int64    `json:"into_tag_version,omitempty"`
}

func tagMergeRequest(approval crmcontracts.Approval) (tagMergePreviewRequest, error) {
	request, err := commercialRequest[tagMergePreviewRequest](approval)
	if err != nil {
		return request, err
	}
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil || *approval.TargetEntityType != "tag" || approval.TargetVersion == nil || approval.ProposedChange == nil {
		return request, apperrors.ErrInvalidArgument
	}
	id := ids.UUID(*approval.TargetEntityId)
	if _, rest := (*approval.ProposedChange)["operation"]; rest {
		if !previewRouteMatches(*approval.ProposedChange, "merge_tags", "tag", id, "POST") {
			return request, apperrors.ErrInvalidArgument
		}
		if !request.SourceID.IsZero() && request.SourceID != id {
			return request, apperrors.ErrInvalidArgument
		}
		request.SourceID = id
	}
	if request.SourceID != id {
		return request, apperrors.ErrInvalidArgument
	}
	return request, nil
}

func (r approvalPreviewReader) tagMergePreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	request, err := tagMergeRequest(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	scope, err := collections.NewStore(r.db).PreviewTagMerge(ctx, ids.From[ids.TagKind](request.SourceID), ids.From[ids.TagKind](request.TargetID), new(int64(*approval.TargetVersion)), request.TargetVersion)
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
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("merge_tag_source", scope.SourceName), previewDetail("merge_tag_target", scope.TargetName)}
	for _, assignment := range scope.Assignments {
		value, err := r.referenceValue(ctx, assignment.EntityType+"_id", assignment.EntityID.String())
		if previewUnavailable(err) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		field := "merge_tag_move"
		if assignment.Collapsed {
			field = "merge_tag_collapse"
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: field, Value: value})
	}
	out.Details, out.Effects, out.State = &details, &[]string{"merge_tags_release_source"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}
