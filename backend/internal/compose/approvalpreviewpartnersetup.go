// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const operationSetUpPartner = "set_up_partner"

type partnerSetUpPreviewRequest struct {
	PartnerRole       string  `json:"partner_role"`
	RelationshipStage *string `json:"relationship_stage"`
	NextStep          *string `json:"next_step"`
	NextStepDueAt     *string `json:"next_step_due_at"`
}

// partnerSetUpPreview shows the terms a program would start with. The margin
// tier is listed as an effect, because it is the part left for a human.
// The company's version does not move when a program is set up. So the preview
// asks the store whether one exists now, and a program set up while the
// approval waited makes it not ready.
func (r approvalPreviewReader) partnerSetUpPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	if approval.TargetEntityId == nil || approval.TargetVersion == nil {
		return out, nil
	}
	request, err := commercialRequest[partnerSetUpPreviewRequest](approval)
	if previewUnavailable(err) || (err == nil && request.PartnerRole == "") {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	company := ids.UUID(*approval.TargetEntityId)
	record, err := r.records.Read(ctx, datasource.EntityRef{Type: datasource.EntityCompany, ID: company})
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.CurrentVersion = new(int(record.Version))
	if record.Version != int64(*approval.TargetVersion) {
		out.State = previewStaleState
		return out, nil
	}
	err = contacts.NewStore(r.db).CheckPartnerSetUp(ctx, ids.From[ids.CompanyKind](company))
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("partner_role", request.PartnerRole)}
	for field, value := range map[string]*string{
		"partner_stage": request.RelationshipStage, "partner_next_step": request.NextStep, "partner_next_step_due_at": request.NextStepDueAt,
	} {
		if value != nil {
			details = append(details, previewDetail(field, *value))
		}
	}
	out.Details, out.Effects, out.State = &details, &[]string{"partner_margin_tier_unset"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}
