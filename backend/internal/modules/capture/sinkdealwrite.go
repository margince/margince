// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// linkActivityToDeal writes the one deal link the attribution rule concluded,
// and answers whether a link landed.
//
// It needs activity.update for the reason linkActivityToProject does, and a
// denial or an unreachable deal writes nothing and is no fault. The deal is
// locked in the shared last-activity order before the target probe, because
// the link's trigger moves the deal's last activity.
func linkActivityToDeal(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, dealID ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	if err := storekit.LockLastActivityTargets(ctx, tx, storekit.LastActivityTargets{Deals: []ids.UUID{dealID}}); err != nil {
		return false, fmt.Errorf("capture: %w", err)
	}
	if err := auth.EnsureLinkTarget(ctx, tx, string(datasource.EntityDeal), dealID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("capture: deal link target: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, deal_id)
		VALUES ($1, 'deal', $2)
		ON CONFLICT DO NOTHING`, activityID, dealID)
	if err != nil {
		return false, fmt.Errorf("capture: filing the activity under its deal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, recordAttributedLink(ctx, tx, activityID, datasource.EntityDeal, dealID)
}

// recordAttributedLink is the bookkeeping an automatic filing owes once its
// link landed. It bumps the version a staged approval re-checks. It writes the
// activity_relink audit row a human relink also writes. The audit row's
// principal tells an automatic link from a human one. No event rides with it:
// activity.captured already announced the message.
func recordAttributedLink(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, entity datasource.EntityType, entityID ids.UUID,
) error {
	// The trigger set_updated_at_bump_version does the bump; this only has to
	// be an UPDATE of the row.
	if _, err := tx.Exec(ctx, `UPDATE activity SET updated_at = now() WHERE id = $1 AND archived_at IS NULL`, activityID); err != nil {
		return fmt.Errorf("capture: bumping the filed activity's version: %w", err)
	}
	_, err := storekit.Audit(ctx, tx, "activity_relink", "activity", activityID.UUID, nil,
		map[string]any{"entity_type": string(entity), "entity_id": entityID})
	return err
}
