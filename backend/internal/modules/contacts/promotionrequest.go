// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// PromotionRequestInput keeps request conversion shared by creation and its approval preview.
func PromotionRequestInput(req crmcontracts.PromoteLeadRequest) PromoteLeadInput {
	in := PromoteLeadInput{Trigger: string(req.Trigger), CreateCompany: req.CreateCompany != nil && *req.CreateCompany}
	in.ExistingDealID = uuidPtrToIDs(req.ExistingDealId)
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

// PromotionDealInput keeps machine choices on the same qualification writer.
func PromotionDealInput(choice datasource.PromotionDealChoice) PromoteLeadInput {
	in := PromoteLeadInput{ExistingDealID: choice.ExistingDealID, CreateCompany: choice.CreateCompany}
	if choice.Deal != nil {
		in.Deal = &QualifyDealInput{PipelineID: choice.Deal.PipelineID, StageID: choice.Deal.StageID, Name: choice.Deal.Name, AmountMinor: choice.Deal.AmountMinor, Currency: choice.Deal.Currency}
	}
	return in
}
