// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type recordMergePreviewRequest struct {
	RecordType string   `json:"record_type"`
	SourceID   ids.UUID `json:"source_id"`
	TargetID   ids.UUID `json:"target_id"`
}

func recordMergeRequest(approval crmcontracts.Approval) (recordMergePreviewRequest, error) {
	request, err := commercialRequest[recordMergePreviewRequest](approval)
	if err != nil {
		return request, err
	}
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil || approval.TargetVersion == nil || approval.ProposedChange == nil {
		return request, apperrors.ErrInvalidArgument
	}
	kind := *approval.TargetEntityType
	if kind != "contact" && kind != "company" {
		return request, apperrors.ErrInvalidArgument
	}
	if _, rest := (*approval.ProposedChange)["operation"]; rest {
		path, ok := (*approval.ProposedChange)["path"].(string)
		if !ok {
			return request, apperrors.ErrInvalidArgument
		}
		parts := strings.Split(path, "/")
		if len(parts) != 5 {
			return request, apperrors.ErrInvalidArgument
		}
		source, err := ids.Parse(parts[3])
		if err != nil || !previewRouteMatches(*approval.ProposedChange, "merge_records", kind, source, "POST") {
			return request, apperrors.ErrInvalidArgument
		}
		if (!request.SourceID.IsZero() && request.SourceID != source) || (request.RecordType != "" && request.RecordType != kind) {
			return request, apperrors.ErrInvalidArgument
		}
		request.SourceID, request.RecordType = source, kind
	}
	if request.RecordType != kind || request.TargetID != ids.UUID(*approval.TargetEntityId) || request.SourceID.IsZero() || request.SourceID == request.TargetID {
		return request, apperrors.ErrInvalidArgument
	}
	return request, nil
}
