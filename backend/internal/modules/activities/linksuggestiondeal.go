// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withDealFacts adds a suggested deal's value and close date, which help the
// reader pick between deals. The value and its currency go through the deal's
// field masks, as every other read of a deal's money does.
func withDealFacts(ctx context.Context, tx pgx.Tx, suggestion *crmcontracts.LinkSuggestion) error {
	args := []any{ids.UUID(suggestion.EntityId)}
	arg := func(v any) int { args = append(args, v); return len(args) }
	amount, err := auth.MaskedColumnSQL(ctx, "deal", "amount_minor", "d", "amount_minor", arg)
	if err != nil {
		return err
	}
	currency, err := auth.MaskedColumnSQL(ctx, "deal", "currency", "d", "currency", arg)
	if err != nil {
		return err
	}
	var closes *time.Time
	err = tx.QueryRow(ctx, storekit.SQLf(`SELECT %s, %s, d.expected_close_date FROM deal d WHERE d.id = $1`, amount, currency), args...).
		Scan(&suggestion.DealAmountMinor, &suggestion.DealCurrency, &closes)
	if err != nil {
		return fmt.Errorf("activities: reading the suggested deal's value: %w", err)
	}
	if closes != nil {
		suggestion.DealExpectedCloseDate = &openapi_types.Date{Time: *closes}
	}
	return nil
}
