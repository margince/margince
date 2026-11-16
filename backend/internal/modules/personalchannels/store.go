// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rbacObject is the one permission behind a member's connection and its replies.
const rbacObject = "personal_channel"

// connectionEntity names the connection in the audit log.
const connectionEntity = "personal_channel_connection"

// Store reads and writes the caller's own connection, replies and uploads.
type Store struct {
	db  *database.DB
	now func() time.Time
}

// NewStore binds the store to the installation's database.
func NewStore(db *database.DB) *Store {
	return &Store{db: db, now: time.Now}
}

// WithClock returns a copy of the store that reads now as its clock.
func (s *Store) WithClock(now func() time.Time) *Store {
	out := *s
	out.now = now
	return &out
}

// member is the human whose chats these are: the caller, or the human an agent
// acts for on their own passport or connection. Nobody else has a connection.
func member(ctx context.Context) (ids.UUID, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return ids.UUID{}, fmt.Errorf("personal channels: no actor bound: %w", apperrors.ErrPermissionDenied)
	}
	switch {
	case p.Type == principal.PrincipalHuman && !p.UserID.IsZero():
		return p.UserID, nil
	case p.ActsAsItsUser():
		return p.OnBehalfOf, nil
	}
	return ids.UUID{}, fmt.Errorf("personal channels belong to a member; this caller acts for none: %w", apperrors.ErrPermissionDenied)
}

// ensureConnection returns the caller's connection row, creating it off.
func ensureConnection(ctx context.Context, tx pgx.Tx, user ids.UUID) (ids.UUID, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO personal_channel_connection (user_id) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id`, user).Scan(&id)
	return id, err
}

// tx runs fn for the caller's own channels, once the caller passed auth.Require.
func (s *Store) tx(ctx context.Context, fn func(tx pgx.Tx, user ids.UUID) error) error {
	user, err := member(ctx)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error { return fn(tx, user) })
}
