// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func (r approvalPreviewReader) dealDecisionContext(ctx context.Context, approval crmcontracts.Approval, current, changes map[string]json.RawMessage, out *crmcontracts.ApprovalPreview) error {
	if !stagePreviewKind(approval.Kind) || approval.TargetEntityType == nil || *approval.TargetEntityType != string(recordTypeDeal) {
		return nil
	}
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, field := range []string{fieldCompanyID, bulkParamOwner, acceptFieldAmountMinor, previewFieldExpectedArrMinor, dealTargetCurrency} {
		if _, changing := changes[field]; changing {
			continue
		}
		raw, visible := current[field]
		if !visible {
			continue
		}
		value, err := r.value(ctx, field, raw)
		if previewUnavailable(err) {
			if field != bulkParamOwner {
				continue
			}
			value = crmcontracts.ApprovalPreviewValue{Kind: "unavailable_name"}
		} else if err != nil {
			return err
		}
		if err := r.moneyValue(ctx, field, &value, current); err != nil {
			if previewUnavailable(err) {
				continue
			}
			return err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: field, Value: value})
	}
	if len(details) > 0 {
		out.Version, out.Details = 2, &details
	}
	return nil
}
