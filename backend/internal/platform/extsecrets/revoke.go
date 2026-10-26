// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extsecrets

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RevokeUnit deletes every secret a unit holds, at both scopes and including
// the core-held keys, destroys the sealed material, and returns the count. It is
// the core's door for a unit that left the composed set, because the unit's own
// port cannot be called any more.
//
// Each deletion is audited in the row's transaction, like Delete.
func RevokeUnit(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, unit string) (int, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return 0, database.ErrNoWorkspace
	}
	s := &store{unit: unit, pool: pool, vault: vault, log: slog.Default()}
	var refs []keyvault.Ref
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`DELETE FROM extension_secret WHERE extension_name = $1 RETURNING key, user_id, vault_ref`, unit)
		if err != nil {
			return err
		}
		type revoked struct {
			key  string
			user *ids.UserID
		}
		var gone []revoked
		for rows.Next() {
			var r revoked
			var ref string
			if err := rows.Scan(&r.key, &r.user, &ref); err != nil {
				rows.Close()
				return err
			}
			gone = append(gone, r)
			refs = append(refs, keyvault.Ref(ref))
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range gone {
			if err := s.audit(ctx, tx, actionDeleted, r.user, r.key); err != nil {
				return fmt.Errorf("extsecrets: auditing the revocation of %s/%s: %w", unit, r.key, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, ref := range refs {
		keyvault.DeleteDetached(ctx, vault, s.log, ws, ref, "ext-secret-revoked")
	}
	return len(refs), nil
}
