// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// The installation's provider-lookup posture (the settings surface over
// settingsentry.go). Thin by design: the entry carries the default, the RBAC
// object and the audit verb, so this file owns only the shape the HTTP layer
// answers with and the gate on the write.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Settings is the installation's provider posture (the wire shape).
type Settings struct {
	AutomaticLookup bool
}

// SettingsStore is the store over that posture.
type SettingsStore struct {
	settings *settings.Store
}

// NewSettings builds the settings store over the settings mechanism.
func NewSettings(s *settings.Store) *SettingsStore { return &SettingsStore{settings: s} }

// Get reads the posture. The read gate lives on the entry, so there is no
// second gate to keep in step here.
func (s *SettingsStore) Get(ctx context.Context) (Settings, error) {
	automatic, err := settings.Get(ctx, s.settings, AutomaticLookup)
	if err != nil {
		return Settings{}, fmt.Errorf("integrations: reading settings: %w", err)
	}
	return Settings{AutomaticLookup: automatic}, nil
}

// Update applies a sparse patch. A nil field is left unchanged; an unchanged
// value writes nothing and audits nothing.
//
// The update gate is taken HERE, before the empty-patch branch, so a read-only
// seat probing the surface with an empty PATCH gets the 403 its authority
// earns rather than a 200 from the read that follows.
func (s *SettingsStore) Update(ctx context.Context, automaticLookup *bool) (Settings, error) {
	if err := auth.Require(ctx, objectIntegrations, principal.ActionUpdate); err != nil {
		return Settings{}, err
	}
	if automaticLookup != nil {
		if err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
			return settings.SetTx(ctx, s.settings, tx, AutomaticLookup, *automaticLookup)
		}); err != nil {
			return Settings{}, err
		}
	}
	return s.Get(ctx)
}

// automaticLookupEnabled is the ONE reader admission and the catch-up sweep
// share, so the two cannot answer the question differently. Ungated by
// declaration (see AutomaticLookup): machinery binding its own write.
func automaticLookupEnabled(ctx context.Context, tx pgx.Tx) (bool, error) {
	on, err := settings.ApplyTx(ctx, tx, AutomaticLookup)
	if err != nil {
		return false, fmt.Errorf("integrations: reading the automatic-lookup posture: %w", err)
	}
	return on, nil
}
