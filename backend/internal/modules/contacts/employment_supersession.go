// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A newer snapshot withdraws a provider's assertion of current employment; it
// cannot invent a departure date or override a human's correction.
func retireSupersededEmployment(ctx context.Context, tx pgx.Tx, id ids.UUID, provider string) error {
	if err := auth.Require(ctx, tableRelationship, principal.ActionUpdate); err != nil {
		return err
	}
	if _, err := storekit.LockRow(ctx, tx, tableRelationship, id, storekit.LiveOnly); err != nil {
		return err
	}
	args := []any{id}
	current, err := scanRelationship(tx.QueryRow(ctx, storekit.SQLf("SELECT %s FROM relationship WHERE id=$%d", relationshipColumns, len(args)), args...))
	if err != nil {
		return err
	}
	if current.CapturedBy != connectorCapturedBy(provider) || (current.EmploymentStatus != nil && *current.EmploymentStatus != employmentCurrent) {
		return nil
	}
	unknown, primary := employmentUnknown, false
	updated, err := patchRelationshipRow(ctx, tx, id, UpdateRelationshipInput{EmploymentStatus: &unknown, IsCurrentPrimary: &primary}, current.CapturedBy)
	if err != nil {
		return err
	}
	if err := emitRelationshipChange(ctx, tx, "update", relationshipFieldImage(current), updated); err != nil {
		return err
	}
	if current.ContactID != nil {
		return promoteLoneSurvivingEmployment(ctx, tx, *current.ContactID)
	}
	return nil
}
