// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Seed writes a bootstrap value once (ADR-0061). It inserts only when no row
// exists, so a restart never overwrites a later human change. It takes no RBAC
// gate, because bootstrap runs before any human exists.
//
// It reports whether the row was stored. `setting` is not tenant-scoped, so a
// re-bootstrap keeps the old rows and discards the operator's new values.
//
// A key in skip is left alone and reported not stored. A boot source sets it,
// so a row written now would take effect only once that source is removed.
func Seed(ctx context.Context, tx pgx.Tx, def Definition, raw json.RawMessage, skip map[string]bool) (stored bool, err error) {
	if skip[def.Key()] {
		return false, nil
	}
	if err := def.ValidateJSON(raw); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx,
		`INSERT INTO setting (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING`,
		def.Key(), raw)
	if err != nil {
		return false, fmt.Errorf("settings: seeding %s: %w", def.Key(), err)
	}
	return tag.RowsAffected() == 1, nil
}

// SeedValue is the typed form of Seed, for a caller holding the entry. It
// reports what Seed reports: whether the value was stored, or discarded because
// a row was already there.
func SeedValue[T any](ctx context.Context, tx pgx.Tx, e *Entry[T], v T, skip map[string]bool) (stored bool, err error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return false, fmt.Errorf("settings: encoding seed for %s: %w", e.Key(), err)
	}
	return Seed(ctx, tx, e, raw, skip)
}

// ResetConfig restores the configuration settings to first-boot state. It
// deletes their rows, since an absent row reads as the registered default.
// Identity settings are spared. Deleting keeps "has anyone changed this?"
// answerable, which a row holding the default would not. A boot-locked key's
// row goes too: first boot never seeds one.
func ResetConfig(ctx context.Context, tx pgx.Tx, reg *Registry) error {
	keys := make([]string, 0, len(reg.byKey))
	for key, def := range reg.byKey {
		if def.SurvivesDataReset() {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM setting WHERE key = ANY($1)`, keys); err != nil {
		return fmt.Errorf("settings: restoring configuration to first-boot state: %w", err)
	}
	return nil
}
