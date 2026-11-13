// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LoggedByTx is the member who put a live activity here: by their own write,
// or through an account they connected. Zero means no member did.
func LoggedByTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID) (ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ids.Nil, err
	}
	var stamp string
	err := tx.QueryRow(ctx, `SELECT captured_by FROM activity WHERE id = $1 AND archived_at IS NULL`, activityID).Scan(&stamp)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("activities: reading who logged an activity: %w", err)
	}
	human := actingHumanOf(stamp)
	if human == "" {
		return ids.Nil, nil
	}
	return ids.Parse(human)
}
