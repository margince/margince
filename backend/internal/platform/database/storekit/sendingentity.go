// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Which legal entity a user works for, and so sends mail and issues offers for.
// contacts owns the tables. consent, contacts and activities all ask this, so
// the one answer lives here where each of them can reach it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SenderOf is the user a message goes out for: the human, or the human an agent
// works for on their passport. Automatic work names nobody.
func SenderOf(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		return ids.Nil
	}
	if actor.Type == principal.PrincipalHuman {
		return actor.UserID
	}
	return actor.OnBehalfOf
}

// LegalEntityOf is the legal entity user works for: the live entity their
// legal_entity_member row names, else the installation's default entity. A
// zero user, and a member of an archived entity, get the default.
func LegalEntityOf(ctx context.Context, tx pgx.Tx, user ids.UUID) (ids.UUID, error) {
	var member *ids.UUID
	if !user.IsZero() {
		member = &user
	}
	args := []*ids.UUID{member}
	var entity *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT coalesce(
		    (SELECT le.id FROM legal_entity_member m
		       JOIN legal_entity le ON le.id = m.legal_entity_id AND le.archived_at IS NULL
		      WHERE m.user_id = `+Placeholders(args)+`),
		    (SELECT id FROM legal_entity WHERE is_default))`, member).Scan(&entity)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("read the legal entity a user works for: %w", err)
	}
	if entity == nil {
		return ids.UUID{}, errors.New("this installation has no default legal entity")
	}
	return *entity, nil
}
