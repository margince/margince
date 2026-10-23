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

// DealFiler is the activities module's automatic deal filing, which owns
// activity_link and the relink event its consumers read. Compose injects it.
type DealFiler interface {
	// FileOnDealIfUnfiled writes the link under the activity's lock only if
	// it still has no deal link. It answers whether the link landed, and
	// whether a deal link was already there.
	FileOnDealIfUnfiled(ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, evidence map[string]any) (landed, filed bool, err error)
	// RetractMachineDeal takes one deal link off under the activity's lock,
	// only while no human or agent ever filed the activity on a deal.
	RetractMachineDeal(ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, evidence map[string]any) (bool, error)
}

// linkActivityToDeal writes the one deal link the attribution rule concluded.
// The decider needs activity.update, as for a project link, and must be able
// to reach the deal; a refusal writes nothing and is no fault. A deal link
// that landed first is kept and reported as the filing it is.
func linkActivityToDeal(f dealFiling, tx pgx.Tx, activityID ids.ActivityID, dealID ids.UUID) (DealOutcome, error) {
	if may, err := mayFileOnDeal(f, tx, dealID); err != nil || !may {
		return DealNotWritable, err
	}
	landed, filed, err := f.filer.FileOnDealIfUnfiled(f.writer, tx, activityID, dealID, f.evidence)
	switch {
	case err != nil:
		return "", err
	case landed:
		return DealLinked, nil
	case filed:
		return DealAlreadyFiled, nil
	default:
		return DealNotWritable, nil
	}
}

// retractMachineDealLink takes a deal link the rule wrote off the activity,
// when the rule no longer finds that deal. Only the repair calls it.
func retractMachineDealLink(f dealFiling, tx pgx.Tx, activityID ids.ActivityID, dealID ids.UUID) (bool, error) {
	if err := auth.Require(f.decider, "activity", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	return f.filer.RetractMachineDeal(f.writer, tx, activityID, dealID, f.evidence)
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
	// An archived or closed deal takes no new mail, whoever may see it.
	if !auth.ReadGranted(f.decider, "deal") {
		return false, nil
	}
	var open bool
	if err := tx.QueryRow(f.writer, `SELECT EXISTS (SELECT 1 FROM deal d
		 WHERE d.id = $1 AND d.status = 'open' AND d.archived_at IS NULL)`, dealID).Scan(&open); err != nil || !open {
		return false, err
	}
	if err := auth.EnsureLinkTarget(f.decider, tx, string(datasource.EntityDeal), dealID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("capture: deal link target: %w", err)
	}
	return true, nil
}
