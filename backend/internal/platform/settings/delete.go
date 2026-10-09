// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DeleteRawTx removes a setting's row, so it reads as its registered default
// again, and audits the removal in the caller's transaction. It reports whether
// a row was there; an absent row is a no-op and writes no audit row.
//
// A removal is a change, so it takes the update gate and freeze probe a write
// takes. It is the only way to unset an entry whose validator refuses the
// default as a stored value, which a sealed reference's validator does.
func (s *Store) DeleteRawTx(ctx context.Context, tx pgx.Tx, key string) (bool, error) {
	def, err := s.lookup(key)
	if err != nil {
		return false, err
	}
	if err := auth.Require(ctx, def.Object(), principal.ActionUpdate); err != nil {
		return false, err
	}
	if err := LockForWrite(ctx, tx, key); err != nil {
		return false, err
	}
	var before json.RawMessage
	err = tx.QueryRow(ctx, `SELECT value FROM setting WHERE key = $1`, key).Scan(&before)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("settings: reading %s before removal: %w", key, err)
	}
	frozen, why, err := def.Frozen(ctx, tx)
	if err != nil {
		return false, fmt.Errorf("settings: probing %s: %w", key, err)
	}
	if frozen {
		return false, FrozenValue{Setting: key, Reason: why}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM setting WHERE key = $1`, key); err != nil {
		return false, fmt.Errorf("settings: removing %s: %w", key, err)
	}
	after, err := def.DefaultJSON()
	if err != nil {
		return false, err
	}
	if _, err := storekit.Audit(ctx, tx, def.AuditVerb(), def.Object(), storekit.MustWorkspace(ctx),
		map[string]any{key: def.AuditImage(before)},
		map[string]any{key: def.AuditImage(after)}); err != nil {
		return false, fmt.Errorf("settings: auditing the removal of %s: %w", key, err)
	}
	return true, nil
}

// DeleteTx is the typed face of DeleteRawTx, for a caller holding the entry.
func DeleteTx[T any](ctx context.Context, s *Store, tx pgx.Tx, e *Entry[T]) (bool, error) {
	return s.DeleteRawTx(ctx, tx, e.Key())
}
