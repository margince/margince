// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/auditentity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const memberListsPredicate = "(owner_id = @member OR steward_id = @member) AND archived_at IS NULL"

// MemberListsCountTx includes private lists in the administrator's handover count.
func MemberListsCountTx(ctx context.Context, tx pgx.Tx, member ids.UserID) (int, error) {
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return 0, err
	}
	var count int
	err := tx.QueryRow(ctx, "SELECT count(*) FROM list WHERE "+memberListsPredicate, pgx.StrictNamedArgs{"member": member}).Scan(&count)
	return count, err
}

// TransferMemberListsTx is the collections write used by member handover.
func TransferMemberListsTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (int, error) {
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, "SELECT id FROM list WHERE "+memberListsPredicate+" ORDER BY id FOR UPDATE", pgx.StrictNamedArgs{"member": from})
	if err != nil {
		return 0, err
	}
	owned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, err
	}
	for _, id := range owned {
		if err := transferMemberListTx(ctx, tx, id, from, to); err != nil {
			return 0, err
		}
	}
	return len(owned), nil
}

func transferMemberListTx(ctx context.Context, tx pgx.Tx, id ids.UUID, from, to ids.UserID) error {
	current, err := scanList(tx.QueryRow(ctx, selectList, listByID(ids.From[ids.ListKind](id))))
	if err != nil {
		return err
	}
	patch := storekit.NewPatch()
	if current.OwnerID != nil && current.OwnerID.UUID == from.UUID {
		patch.Set("owner_id", from.UUID, to.UUID)
	}
	if current.StewardID != nil && current.StewardID.UUID == from.UUID {
		patch.Set("steward_id", from.UUID, to.UUID)
	}
	if patch.Empty() {
		return nil
	}
	if err := patch.ApplyGuarded(ctx, tx, listObject, id, &current.Version); err != nil {
		return err
	}
	after, err := scanList(tx.QueryRow(ctx, selectList, listByID(ids.From[ids.ListKind](id))))
	if err != nil {
		return err
	}
	if err := writeRevision(ctx, tx, after); err != nil {
		return err
	}
	auditID, err := storekit.Audit(ctx, tx, "update", auditentity.List, id, patch.Before(), patch.After())
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventListUpdated{Version: after.Version, Changed: changedKeys(patch.After())})
}

// ArchiveMemberSavedViewsTx removes personal settings when their member leaves.
func ArchiveMemberSavedViewsTx(ctx context.Context, tx pgx.Tx, owner ids.UserID) error {
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM saved_view WHERE owner_id = @owner AND archived_at IS NULL ORDER BY id FOR UPDATE`, pgx.StrictNamedArgs{"owner": owner})
	if err != nil {
		return err
	}
	archived, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, id := range archived {
		if _, err := archiveSavedViewTx(ctx, tx, ids.From[ids.SavedViewKind](id), owner); err != nil {
			return err
		}
	}
	return nil
}
