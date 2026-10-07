// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type bindings struct{ values []any }

//craft:ignore naked-any SQL parameters are heterogeneous values encoded by pgx, never interpolated identifiers.
func (b *bindings) add(value any) string {
	b.values = append(b.values, value)
	return fmt.Sprintf("$%d", len(b.values))
}

func actorID(ctx context.Context) (ids.UUID, error) {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return ids.Nil, err
	}
	if !actor.UserID.IsZero() {
		return actor.UserID, nil
	}
	if !actor.OnBehalfOf.IsZero() {
		return actor.OnBehalfOf, nil
	}
	return ids.Nil, fmt.Errorf("reporting authoring requires a human: %w", apperrors.ErrPermissionDenied)
}

func storedError(err error) error {
	if _, unique := storekit.UniqueViolation(err); unique {
		return apperrors.ErrConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	return err
}

func recordChange[T any](ctx context.Context, tx pgx.Tx, object string, id ids.UUID, action string, before *T, after T) error {
	if action != "create" && before == nil {
		return fmt.Errorf("reporting mutation requires its previous image")
	}
	audit, err := storekit.Audit(ctx, tx, action, object, id, before, after)
	if err != nil {
		return err
	}
	payload := struct {
		Object string `json:"object"`
		Action string `json:"action"`
	}{object, action}
	return storekit.Emit(ctx, tx, audit, "reporting.changed", object, id, payload)
}

func requireReadWrite(ctx context.Context, object string, action principal.Action) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || !actor.SeatType.CanMutate() {
		return apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return err
	}
	return auth.Require(ctx, object, action)
}

func encode[T any](value T) ([]byte, error) { return json.Marshal(value) }
