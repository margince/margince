// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A handoff inherits its prospect's scope; assigning work never grants access.
func ensureHandoffSubjectWritable(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, _, err := handoffSubject(ctx, tx, id)
	return err
}

const handoffSubjectSQL = `SELECT lead_id, contact_id FROM sdr_handoff WHERE id = $1`

func handoffSubject(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, ids.UUID, error) {
	entity, subject, err := decodeHandoffSubject(tx.QueryRow(ctx, handoffSubjectSQL, id))
	if err != nil {
		return "", ids.UUID{}, err
	}
	if err := auth.Require(ctx, entity, principal.ActionRead); err != nil {
		return "", ids.UUID{}, err
	}
	if err := auth.Require(ctx, entity, principal.ActionUpdate); err != nil {
		return "", ids.UUID{}, err
	}
	return entity, subject, auth.EnsureWritable(ctx, tx, entity, subject)
}

func readableHandoffSubject(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	entity, subject, err := decodeHandoffSubject(tx.QueryRow(ctx, handoffSubjectSQL, id))
	if err != nil {
		return err
	}
	if err := auth.Require(ctx, entity, principal.ActionRead); err != nil {
		return err
	}
	return auth.EnsureVisibleExisting(ctx, tx, entity, subject)
}

func decodeHandoffSubject(row pgx.Row) (string, ids.UUID, error) {
	var leadID, contactID *ids.UUID
	err := row.Scan(&leadID, &contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ids.UUID{}, apperrors.ErrNotFound
	}
	if err != nil {
		return "", ids.UUID{}, fmt.Errorf("contacts: reading the handoff's subject: %w", err)
	}
	if leadID != nil {
		return leadEntity, *leadID, nil
	}
	if contactID != nil {
		return entityContact, *contactID, nil
	}
	return "", ids.UUID{}, errHandoffNeedsOneSubject
}
