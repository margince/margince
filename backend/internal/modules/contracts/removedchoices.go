// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// RemoveCustomChoices clears choices an admin removed from a contract field off
// every contract that holds one. Each goes through applyContractUpdate, the
// writer of a contract update, so it is audited and announced as an edit is.
// Archived contracts are cleared too, because the field's CHECK covers them.
func RemoveCustomChoices(ctx context.Context, tx pgx.Tx, column fieldcatalog.Column, removed []string) (int, error) {
	changes, err := storekit.LockRemovedChoices(ctx, tx, contractTable, column, removed)
	if err != nil {
		return 0, err
	}
	only := []fieldcatalog.Column{{Name: column.Name, Type: column.Type}}
	for _, change := range changes {
		patch := storekit.NewPatch()
		if err := storekit.SetCustomFieldPatch(patch, only,
			map[string]any{column.Name: change.After}, map[string]any{column.Name: change.Before}); err != nil {
			return 0, err
		}
		if err := applyContractUpdate(ctx, tx, ids.ContractID{UUID: change.ID}, patch, nil, storekit.IncludeArchived, "removed choice"); err != nil {
			return 0, err
		}
	}
	return len(changes), nil
}
