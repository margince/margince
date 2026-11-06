// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// pendingWrite is one field of a sparse patch, already reduced to the setting
// it writes, its JSON, and the write itself. `raw` nil means the field was
// absent and nothing is written.
type pendingWrite struct {
	key  string
	raw  []byte
	save func(ctx context.Context, tx pgx.Tx, s *InstallationSettingsStore) error
}

// encodePatchField reduces one patch field to a pendingWrite, encoding it here
// where its type is still known rather than handing the loop an `any`.
//
// Generic over the field's own type, which keeps the write typed. The
// installation's fields are strings except the fiscal start month. A loop
// carrying `any` would accept a value of any type at all. That includes, silently,
// a nil typed pointer, which is not equal to nil once it is inside an
// interface.
func encodePatchField[T any](entry settings.Definition, value *T) (pendingWrite, error) {
	if value == nil {
		return pendingWrite{key: entry.Key()}, nil
	}
	raw, err := json.Marshal(*value)
	if err != nil {
		return pendingWrite{}, fmt.Errorf("identity: encoding %s: %w", entry.Key(), err)
	}
	return pendingWrite{key: entry.Key(), raw: raw, save: func(ctx context.Context, tx pgx.Tx, s *InstallationSettingsStore) error {
		return s.settings.SetRawTx(ctx, tx, entry.Key(), raw)
	}}, nil
}

// encodeStoredField is encodePatchField for a setting in the configuration
// registry, whose save refuses a value a boot source sets.
func encodeStoredField[T any](setting *config.Setting[T], value *T) (pendingWrite, error) {
	if value == nil {
		return pendingWrite{key: setting.Key()}, nil
	}
	raw, err := json.Marshal(*value)
	if err != nil {
		return pendingWrite{}, fmt.Errorf("identity: encoding %s: %w", setting.Key(), err)
	}
	next := *value
	return pendingWrite{key: setting.Key(), raw: raw, save: func(ctx context.Context, tx pgx.Tx, s *InstallationSettingsStore) error {
		return config.SaveTx(ctx, s.config, tx, setting, next)
	}}, nil
}
