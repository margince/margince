// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	memberSourceArgument = "source"
	memberOwnerField     = "owner_id"
)

// TransferMemberSignalsTx carries pending alerts with a consolidated identity.
// Closed alerts retain their original attribution.
func TransferMemberSignalsTx(ctx context.Context, tx pgx.Tx, source, target ids.UUID) error {
	if err := auth.Require(ctx, "user_admin", principal.ActionDelete); err != nil {
		return err
	}
	if err := auth.Require(ctx, "signal", principal.ActionUpdate); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM signal WHERE owner_id = @source AND status = 'open' AND archived_at IS NULL ORDER BY id FOR UPDATE`, pgx.StrictNamedArgs{memberSourceArgument: source})
	if err != nil {
		return err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, id := range found {
		if _, err := tx.Exec(ctx, `UPDATE signal SET owner_id = @target, version = version + 1 WHERE id = @id`, pgx.StrictNamedArgs{"id": id, "target": target}); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "signal", id, map[string]ids.UUID{memberOwnerField: source}, map[string]ids.UUID{memberOwnerField: target})
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventSignalUpdated{SignalId: openapi_types.UUID(id), OwnerId: openapi_types.UUID(target)}); err != nil {
			return err
		}
	}
	return nil
}

// ArchiveMemberSignalsTx retires personal alerts without granting them to a successor.
func ArchiveMemberSignalsTx(ctx context.Context, tx pgx.Tx, source ids.UUID) error {
	if err := auth.Require(ctx, "user_admin", principal.ActionDelete); err != nil {
		return err
	}
	if err := auth.Require(ctx, "signal", principal.ActionDelete); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM signal WHERE owner_id = @source AND status = 'open' AND archived_at IS NULL ORDER BY id FOR UPDATE`, pgx.StrictNamedArgs{memberSourceArgument: source})
	if err != nil {
		return err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, id := range found {
		if err := archiveSignalTx(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}
