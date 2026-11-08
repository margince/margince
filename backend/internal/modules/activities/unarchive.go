// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var activityUnarchive = storekit.UnarchiveShape{
	Table:    linkEntityActivity,
	Lock:     `SELECT COALESCE(subject,''),archived_at,false,version FROM activity WHERE id=$1 AND restricted_at IS NULL FOR UPDATE`,
	Restored: crmcontracts.PublicEventActivityUpdated{ChangedFields: crmcontracts.PublicEventActivityChangedFields{Restored: new(true)}},
}

// RestoreActivityTx reverses an archive under the same audience and write
// authority as the activity. Capture identity and source content remain intact.
func (s *Store) RestoreActivityTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, ifVersion *int64, with storekit.RestoreWith) (storekit.RestoreReport, error) {
	if err := auth.Require(ctx, "activity", principal.ActionDelete); err != nil {
		return storekit.RestoreReport{}, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.IncludeArchived); err != nil {
		return storekit.RestoreReport{}, err
	}
	if err := auth.EnsureActivityContentVisible(ctx, tx, id.UUID); err != nil {
		return storekit.RestoreReport{}, err
	}
	if err := auth.EnsureActivityWritableIn(ctx, tx, id.UUID, false); err != nil {
		return storekit.RestoreReport{}, err
	}
	if err := refuseActiveInvitationPatch(ctx, tx, id.UUID); err != nil {
		return storekit.RestoreReport{}, err
	}
	return storekit.Unarchive(ctx, tx, activityUnarchive, id.UUID, ifVersion, with)
}
