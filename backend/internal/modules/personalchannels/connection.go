// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannels

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ErrOff refuses a channel call while the member has their connection off.
var ErrOff = fmt.Errorf("personal channels are off; turn them on in Settings → Connections: %w", apperrors.ErrConflict)

// Connection is the caller's switch and the replies waiting on it.
type Connection struct {
	ID                   ids.UUID
	Version              int64
	Enabled              bool
	LastPullAt           *time.Time
	RepliesWaiting       int
	OldestReplyWaitingAt *time.Time
}

const connectionSQL = `
	SELECT c.id, c.version, c.enabled, c.last_pull_at,
	       count(r.id) FILTER (WHERE r.state IN ('waiting', 'pulled')),
	       min(r.created_at) FILTER (WHERE r.state IN ('waiting', 'pulled'))
	  FROM personal_channel_connection c
	  LEFT JOIN personal_channel_reply r ON r.connection_id = c.id
	 WHERE c.user_id = $1
	 GROUP BY c.id`

func readConnection(ctx context.Context, tx pgx.Tx, user ids.UUID) (Connection, error) {
	var c Connection
	err := tx.QueryRow(ctx, connectionSQL, user).
		Scan(&c.ID, &c.Version, &c.Enabled, &c.LastPullAt, &c.RepliesWaiting, &c.OldestReplyWaitingAt)
	return c, err
}

// Connection reads the caller's connection, creating it off on first read.
func (s *Store) Connection(ctx context.Context) (Connection, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return Connection{}, err
	}
	var out Connection
	err := s.tx(ctx, func(tx pgx.Tx, user ids.UUID) error {
		if _, err := ensureConnection(ctx, tx, user); err != nil {
			return err
		}
		var err error
		out, err = readConnection(ctx, tx, user)
		return err
	})
	return out, err
}

// SetEnabled turns the caller's connection on or off.
func (s *Store) SetEnabled(ctx context.Context, enabled bool) (Connection, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return Connection{}, err
	}
	var out Connection
	err := s.tx(ctx, func(tx pgx.Tx, user ids.UUID) error {
		if _, err := ensureConnection(ctx, tx, user); err != nil {
			return err
		}
		before, err := readConnection(ctx, tx, user)
		if err != nil {
			return err
		}
		if before.Enabled == enabled {
			out = before
			return nil
		}
		tag, err := tx.Exec(ctx, `
			UPDATE personal_channel_connection
			   SET enabled = $2, version = version + 1, updated_at = now()
			 WHERE id = $1 AND version = $3`, before.ID, enabled, before.Version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("the connection changed while it was being switched; try again: %w", apperrors.ErrConflict)
		}
		if _, err := storekit.Audit(ctx, tx, "update", connectionEntity, before.ID,
			map[string]any{"enabled": before.Enabled}, map[string]any{"enabled": enabled}); err != nil {
			return err
		}
		out, err = readConnection(ctx, tx, user)
		return err
	})
	return out, err
}

// RequireEnabled is the caller's connection when it is on, for a channel call
// inside tx; ErrOff when it is off.
func (s *Store) RequireEnabled(ctx context.Context, tx pgx.Tx, user ids.UUID) (Connection, error) {
	c, err := readConnection(ctx, tx, user)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !c.Enabled) {
		return Connection{}, ErrOff
	}
	return c, err
}
