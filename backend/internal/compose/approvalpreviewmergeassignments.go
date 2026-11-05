// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assignments"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) companyMergeAssignmentDetails(ctx context.Context, source, target ids.UUID) ([]crmcontracts.ApprovalPreviewDetail, error) {
	store := assignments.NewStore(r.db)
	from, err := store.ListForRecord(ctx, crmcontracts.AssignmentRecordTypeCompany, source)
	if err != nil {
		return nil, err
	}
	to, err := store.ListForRecord(ctx, crmcontracts.AssignmentRecordTypeCompany, target)
	if err != nil {
		return nil, err
	}
	if len(from) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	type assignmentKey struct {
		role, subject openapi_types.UUID
		kind          crmcontracts.AssignmentSubjectKind
	}
	kept := map[assignmentKey]bool{}
	for _, assignment := range to {
		kept[assignmentKey{assignment.RoleId, assignment.SubjectId, assignment.SubjectKind}] = true
	}
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, assignment := range from {
		if assignment.SubjectName == nil || assignment.RoleLabel == nil {
			return nil, apperrors.ErrInvalidArgument
		}
		field := "merge_assignment_moved"
		if kept[assignmentKey{assignment.RoleId, assignment.SubjectId, assignment.SubjectKind}] {
			field = "merge_assignment_collapsed"
		}
		detail := mergeIdentityDetail(field, *assignment.SubjectName, ids.UUID(assignment.SubjectId))
		detail.Value.Kind, detail.Value.EntityType = previewValueReference, new(string(assignment.SubjectKind))
		details = append(details, detail, mergeIdentityDetail("merge_assignment_role", *assignment.RoleLabel, ids.UUID(assignment.RoleId)))
	}
	return details, nil
}
