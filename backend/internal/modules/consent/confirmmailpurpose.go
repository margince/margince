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
	return purposeName(key, label, words), nil
}

// purposeName is a purpose's name in one language. The three purposes the
// product seeds take the catalog's name; any other keeps its operator's label.
// The mail, the page heading and the recorded consent sentence all use it.
func purposeName(key, label string, words mailcopy.Copy) string {
	switch key {
	case "business_correspondence":
		return words.BusinessPurpose
	case "marketing_email":
		return words.MarketingPurpose
	case "transactional":
		return words.TransactionalPurpose
	}
	return label
}
