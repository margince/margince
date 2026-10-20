// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func (r approvalPreviewReader) moneyValue(ctx context.Context, field string, value *crmcontracts.ApprovalPreviewValue, fields map[string]json.RawMessage) error {
	if value.Kind != previewValueNumber {
		return nil
	}
	var currency string
	switch field {
	case acceptFieldAmountMinor, "expected_arr_minor", "unit_price_minor", "net_minor", "tax_minor", "gross_minor", "arr_minor", "net_tcv_minor", "line_net_minor", "line_tax_minor", "line_total_minor":
		if err := json.Unmarshal(fields["currency"], &currency); err != nil || currency == "" {
			return apperrors.ErrInvalidArgument
		}
	case "amount_minor_base":
		if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			currency, err = DealsInstallation().BaseCurrency(ctx, tx)
			return err
		}); err != nil {
			return err
		}
	default:
		return nil
	}
	value.Kind, value.Currency = "money", &currency
	return nil
}
