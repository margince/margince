// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// claimDraftStarter gives an unowned draft template the enabling person's
// authority. Existing owners never change when somebody toggles the template.
func claimDraftStarter(ctx context.Context, tx pgx.Tx, before Automation) (ids.UUID, error) {
	entry, ok := CatalogEntryByKey(before.Key)
	if !ok {
		return ids.Nil, fmt.Errorf("automation %s names an unknown catalog key", before.ID)
	}
	if entry.Action != string(ActionTypeDraftEmail) {
		return ids.Nil, nil
	}
	if err := requireAuthorCeiling(ctx, entry); err != nil {
		return ids.Nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ids.Nil, &MissingDraftOwnerError{}
	}
	updated, err := tx.Exec(ctx, `UPDATE automation SET owner_id = @owner
		WHERE id = @id AND owner_id IS NULL`,
		pgx.NamedArgs{"owner": actor.UserID, "id": before.ID})
	if err != nil || updated.RowsAffected() == 0 {
		return ids.Nil, err
	}
	return actor.UserID, nil
}
