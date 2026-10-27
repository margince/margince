// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func mayReadLead(ctx context.Context) (bool, error) {
	err := auth.Require(ctx, leadEntity, principal.ActionRead)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}

func activityIsInbound(ctx context.Context, tx pgx.Tx, activity ids.ActivityID) (bool, error) {
	var inbound bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(direction = 'inbound', false) FROM activity WHERE id = $1 AND archived_at IS NULL`, activity).Scan(&inbound); err != nil {
		return false, fmt.Errorf("contacts: reading a captured activity's direction: %w", err)
	}
	return inbound, nil
}

// linkActivityToLead files a captured message on the lead its sender is.
func linkActivityToLead(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, leadID ids.LeadID) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, lead_id)
		SELECT $1, 'lead', $2
		WHERE NOT EXISTS (
			SELECT 1 FROM activity_link WHERE activity_id = $1 AND entity_type = 'lead' AND lead_id = $2)`,
		activityID, leadID); err != nil {
		return fmt.Errorf("contacts: filing an activity on a lead: %w", err)
	}
	return nil
}
