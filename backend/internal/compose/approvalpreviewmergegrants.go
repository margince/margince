// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

func (r approvalPreviewReader) mergeGrantDetails(ctx context.Context, scope contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, grant := range scope.Grants {
		subject, err := r.referenceValue(ctx, grant.SubjectType+"_id", grant.SubjectID.String())
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "merge_grant_subject", Value: subject}, mergeIdentityDetail("merge_grant_source", grant.Source.Access, grant.SourceID))
		before := authz.RecordGrantState{}
		if grant.Before != nil {
			before = *grant.Before
		}
		details = append(details, previewDetail("merge_grant_before", before.Access), previewMoment("merge_grant_expiry_before", before.ExpiresAt), previewDetail("merge_grant_after", grant.After.Access), previewMoment("merge_grant_expiry_after", grant.After.ExpiresAt), mergeBooleanDetail("merge_grant_active", grant.ActiveAfter))
		if grant.After.Reason != nil {
			details = append(details, previewDetail("merge_grant_reason", *grant.After.Reason))
		}
	}
	return details, nil
}
