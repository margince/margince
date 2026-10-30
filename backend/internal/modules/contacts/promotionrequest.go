// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PromotionRequestInput keeps request conversion shared by creation and its approval preview.
func PromotionRequestInput(req crmcontracts.PromoteLeadRequest) PromoteLeadInput {
	in := PromoteLeadInput{Trigger: string(req.Trigger), CreateCompany: req.CreateCompany != nil && *req.CreateCompany}
	if req.Evidence != nil {
		in.EvidenceNote = req.Evidence.Note
		in.EvidenceActivityID = idArg[ids.ActivityKind](req.Evidence.ActivityId)
	}
	if req.Deal != nil {
		in.Deal = &QualifyDealInput{
			PipelineID: uuidPtrToIDs(req.Deal.PipelineId), StageID: uuidPtrToIDs(req.Deal.StageId),
			AmountMinor: req.Deal.AmountMinor, Currency: req.Deal.Currency,
		}
		if req.Deal.Name != nil {
			in.Deal.Name = *req.Deal.Name
		}
	}

	return in
}
