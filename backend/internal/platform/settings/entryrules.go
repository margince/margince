// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WithAuthorization adds the owning module's restriction to the object grant.
func (e *Entry[T]) WithAuthorization(check func(context.Context, principal.Action) error) *Entry[T] {
	e.authorize = check
	return e
}

// WithChangeCheck checks a proposed change while its stored value is locked.
func (e *Entry[T]) WithChangeCheck(check func(context.Context, pgx.Tx, T, T) error) *Entry[T] {
	e.checkChange = check
	return e
}

// Authorize applies an entry's additional access restriction.
func (e *Entry[T]) Authorize(ctx context.Context, action principal.Action) error {
	if e.authorize == nil {
		return nil
	}
	return e.authorize(ctx, action)
}

// CheckChange includes no-op writes: a matching value cannot bypass a guard.
func (e *Entry[T]) CheckChange(ctx context.Context, tx pgx.Tx, before, next json.RawMessage) error {
	if e.checkChange == nil {
		return nil
	}
	var prior, proposed T
	if err := json.Unmarshal(before, &prior); err != nil {
		return fmt.Errorf("settings: reading the prior %s value: %w", e.key, err)
	}
	if err := json.Unmarshal(next, &proposed); err != nil {
		return fmt.Errorf("settings: reading the proposed %s value: %w", e.key, err)
	}
	return e.checkChange(ctx, tx, prior, proposed)
}

// WithChanged attaches the owner's event to every transport's setting write.
func (e *Entry[T]) WithChanged(emit func(context.Context, pgx.Tx, ids.UUID) error) *Entry[T] {
	e.changed = emit
	return e
}

// Changed runs after the audit, inside the setting's transaction.
func (e *Entry[T]) Changed(ctx context.Context, tx pgx.Tx, auditID ids.UUID) error {
	if e.changed == nil {
		return nil
	}
	return e.changed(ctx, tx, auditID)
}
