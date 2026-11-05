// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/automation"
)

func (r approvalPreviewReader) confirmationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return nil, err
	}
	request, err := automation.ReadConfirmationRequest(raw)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("confirmation_question", request.Question)}
	if request.Reason != "" {
		details = append(details, previewDetail("confirmation_reason", request.Reason))
	}
	out.Details, out.Effects, out.State = &details, &[]string{"record_confirmation_only"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}
