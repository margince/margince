// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func (r approvalPreviewReader) importLinkDetails(ctx context.Context, links []migration.ReviewLink) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var details []crmcontracts.ApprovalPreviewDetail
	for _, link := range links {
		field := "import_link_" + link.Object
		value := readPreviewText(link.Key)
		switch {
		case link.Dropped:
			field = "import_dropped_link"
		case link.Pending:
			field = "import_link_row_" + link.Object
			value = readPreviewText(link.PendingKey)
		default:
			var err error
			value, err = r.referenceValue(ctx, link.Object+"_id", link.TargetID)
			if err != nil {
				return nil, err
			}
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: field, Value: value})
	}
	return details, nil
}

func omitPendingImportDefaults(details []crmcontracts.ApprovalPreviewDetail, links []migration.ReviewLink) []crmcontracts.ApprovalPreviewDetail {
	for _, link := range links {
		if link.Pending {
			details = slices.DeleteFunc(details, func(detail crmcontracts.ApprovalPreviewDetail) bool {
				return detail.Field == link.Object+"_id"
			})
		}
	}
	return details
}

func (r approvalPreviewReader) importPendingCompanyChange(ctx context.Context, object string, current []byte, links []migration.ReviewLink, out *crmcontracts.ApprovalPreviewImportRow) error {
	if object != migration.ObjectDeal {
		return nil
	}
	for _, link := range links {
		if link.Object != migration.ObjectCompany || !link.Pending {
			continue
		}
		var stored map[string]json.RawMessage
		if err := json.Unmarshal(current, &stored); err != nil {
			return err
		}
		beforeRaw := stored["company_id"]
		if len(beforeRaw) == 0 {
			beforeRaw = json.RawMessage("null")
		}
		before, err := r.value(ctx, "company_id", beforeRaw)
		if err != nil {
			return err
		}
		out.Changes = append(out.Changes, crmcontracts.ApprovalPreviewChange{
			Field: "import_link_row_company", Before: before, After: readPreviewText(link.PendingKey),
		})
	}
	return nil
}
