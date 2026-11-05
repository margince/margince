// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func subscriptionPurposeLabel(ctx context.Context, tx pgx.Tx, id ids.PurposeID, words mailcopy.Copy) (string, error) {
	var key, label string
	args := []any{id}
	err := tx.QueryRow(ctx, "SELECT key,label FROM consent_purpose WHERE id = "+storekit.Placeholders(args)+" AND archived_at IS NULL", args...).Scan(&key, &label)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", &ValidationError{Field: "purpose_id", Reason: "choose an active subscription purpose"}
	}
	if err != nil {
		return "", err
	}
	switch key {
	case "business_correspondence":
		return words.BusinessPurpose, nil
	case "marketing_email":
		return words.MarketingPurpose, nil
	case "transactional":
		return words.TransactionalPurpose, nil
	}
	return label, nil
}
