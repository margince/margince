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
// The decider needs activity.update for the reason linkActivityToProject does,
// and must be able to reach the deal. A denial or an unreachable deal writes
// nothing and is no fault. The deal is locked in the shared last-activity order
// first, because the link's trigger moves the deal's last activity.
func linkActivityToDeal(f dealFiling, tx pgx.Tx, activityID ids.ActivityID, dealID ids.UUID) (bool, error) {
	if may, err := mayFileOnDeal(f, tx, dealID); err != nil || !may {
		return false, err
	}
	tag, err := tx.Exec(f.writer, `
		INSERT INTO activity_link (activity_id, entity_type, deal_id)
		VALUES ($1, 'deal', $2)
		ON CONFLICT DO NOTHING`, activityID, dealID)
	if err != nil {
		return false, fmt.Errorf("capture: filing the activity under its deal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, recordLinkChange(f.writer, tx, activityID, nil,
		map[string]any{"entity_type": string(datasource.EntityDeal), "entity_id": dealID}, f.evidence)
}

// retractMachineDealLink takes a deal link the rule wrote off the activity,
// when the rule no longer finds that deal. Only the repair calls it, and only
// for a link whose recorded provenance is the rule's own.
func retractMachineDealLink(f dealFiling, tx pgx.Tx, activityID ids.ActivityID, dealID ids.UUID) (bool, error) {
	if may, err := mayFileOnDeal(f, tx, dealID); err != nil || !may {
		return false, err
	}
	tag, err := tx.Exec(f.writer, `
		DELETE FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal' AND deal_id = $2`, activityID, dealID)
	if err != nil {
		return false, fmt.Errorf("capture: taking a machine deal link back: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, recordLinkChange(f.writer, tx, activityID,
		map[string]any{"entity_type": string(datasource.EntityDeal), "entity_id": dealID}, nil, f.evidence)
}

// mayFileOnDeal is the decider's authority over the activity and the deal,
// with the deal row locked in the shared order.
func mayFileOnDeal(f dealFiling, tx pgx.Tx, dealID ids.UUID) (bool, error) {
	if err := auth.Require(f.decider, "activity", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	if err := storekit.LockLastActivityTargets(f.writer, tx, storekit.LastActivityTargets{Deals: []ids.UUID{dealID}}); err != nil {
		return false, fmt.Errorf("capture: %w", err)
	}
	if err := auth.EnsureLinkTarget(f.decider, tx, string(datasource.EntityDeal), dealID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("capture: deal link target: %w", err)
	}
	return true, nil
}

// recordLinkChange is the bookkeeping an automatic filing owes once its link
// changed. It bumps the version a staged approval re-checks. It writes the
// activity_relink audit row a human relink also writes, with the reason in
// evidence. The audit row's principal tells an automatic link from a human
// one. No event rides with it: activity.captured already announced the message.
func recordLinkChange(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, before, after any, evidence map[string]any) error {
	// The trigger set_updated_at_bump_version does the bump; this only has to
	// be an UPDATE of the row.
	if _, err := tx.Exec(ctx, `UPDATE activity SET updated_at = now() WHERE id = $1 AND archived_at IS NULL`, activityID); err != nil {
		return fmt.Errorf("capture: bumping the filed activity's version: %w", err)
	}
	_, err := storekit.AuditWithEvidence(ctx, tx, "activity_relink", "activity", activityID.UUID, before, after, evidence)
	return err
}
