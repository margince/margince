// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

type communicationPreviewClaim struct {
	category  commsauthz.Category
	purpose   string
	marketing string
	reason    string
	evidence  commsauthz.Evidence
}

func (r approvalPreviewReader) communicationDetails(ctx context.Context, claim communicationPreviewClaim) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("communication_context", string(claim.category))}
	for _, item := range []struct{ field, value string }{
		{"message_purpose", claim.purpose}, {"marketing_purpose", claim.marketing}, {"operator_reason", claim.reason},
	} {
		if item.value != "" {
			details = append(details, previewDetail(item.field, item.value))
		}
	}
	for _, source := range []struct {
		field string
		id    ids.UUID
	}{
		{dealIDField, claim.evidence.DealID}, {previewFieldContractID, claim.evidence.ContractID}, {"invoice_id", claim.evidence.InvoiceID},
	} {
		if source.id == ids.Nil {
			continue
		}
		value, err := r.communicationEvidence(ctx, source.field, source.id)
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: source.field, Value: value})
	}
	return details, nil
}

func (r approvalPreviewReader) communicationEvidence(ctx context.Context, field string, id ids.UUID) (crmcontracts.ApprovalPreviewValue, error) {
	kind := previewReferenceType(field)
	var label string
	switch field {
	case previewFieldContractID:
		agreement, err := contracts.NewStore(r.db, r.deals.FreezeRateAt, ContractTimezone()).GetContract(ctx, ids.From[ids.ContractKind](id))
		if err != nil {
			return crmcontracts.ApprovalPreviewValue{}, err
		}
		label = agreement.Title
	case "invoice_id":
		company, number, err := finance.NewStore(r.db, FinanceRates()).InvoiceReference(ctx, id)
		if err != nil {
			return crmcontracts.ApprovalPreviewValue{}, err
		}
		account, err := r.referenceValue(ctx, refColCompany, company.String())
		if err != nil {
			return crmcontracts.ApprovalPreviewValue{}, err
		}
		label = account.Text
		if number != "" {
			label += " · " + number
		}
	default:
		return r.referenceValue(ctx, field, id.String())
	}
	if strings.TrimSpace(label) == "" {
		return crmcontracts.ApprovalPreviewValue{}, apperrors.ErrPermissionDenied
	}
	return crmcontracts.ApprovalPreviewValue{Kind: previewValueReference, Text: label, EntityType: &kind, EntityId: new(openapi_types.UUID(id))}, nil
}
