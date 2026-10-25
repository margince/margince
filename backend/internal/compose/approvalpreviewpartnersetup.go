// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

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
	CompanyID         *string `json:"company_id"`
	ApprovalID        *string `json:"approval_id"`
}

// decodePartnerSetUpPreview reads an approval's edited payload by the tool's own
// rules: a closed set of members, the contract's vocabularies and a real date.
// An edit that adds a margin tier, or says what execution would refuse, is not
// a payload the preview may call ready.
func decodePartnerSetUpPreview(approval crmcontracts.Approval) (partnerSetUpPreviewRequest, bool, error) {
	var request partnerSetUpPreviewRequest
	args, err := commercialArguments(approval)
	if err != nil {
		return request, false, err
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return request, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, false, nil //nolint:nilerr // A malformed stored request is shown as incomplete.
	}
	if !crmcontracts.SetUpPartnerRequestPartnerRole(request.PartnerRole).Valid() {
		return request, false, nil
	}
	if stage := request.RelationshipStage; stage != nil && !crmcontracts.SetUpPartnerRequestRelationshipStage(*stage).Valid() {
		return request, false, nil
	}
	if due := request.NextStepDueAt; due != nil {
		if _, err := time.Parse(time.DateOnly, *due); err != nil {
			return request, false, nil //nolint:nilerr // A malformed stored due date is shown as incomplete.
		}
	}
	return request, true, nil
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
	request, valid, err := decodePartnerSetUpPreview(approval)
	if previewUnavailable(err) || (err == nil && !valid) {
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
	for _, optional := range []struct {
		field string
		value *string
	}{
		{"partner_stage", request.RelationshipStage}, {"partner_next_step", request.NextStep}, {"partner_next_step_due_at", request.NextStepDueAt},
	} {
		if optional.value != nil {
			details = append(details, previewDetail(optional.field, *optional.value))
		}
	}
	out.Details, out.Effects, out.State = &details, &[]string{"partner_margin_tier_unset"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}
