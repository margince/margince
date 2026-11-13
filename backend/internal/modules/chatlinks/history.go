// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PullLinks reads the active links of a bot connection, oldest first, so the
// pull reads each linked channel's history.
func (s *Store) PullLinks(ctx context.Context, connectionID ids.UUID) ([]Link, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var out []Link
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+linkColumns+` FROM chat_link
			WHERE connection_id = $1 AND archived_at IS NULL AND status = $2
			ORDER BY created_at, id`, connectionID, StatusActive)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) { return scanLink(row) })
		return err
	})
	return out, err
}

// AdvanceHistory moves a link's history cursor from the value the caller read
// to the last message it judged. It answers false when another pull moved the
// cursor first, so that pull's judgement stands and this one stops. backfilled
// clears the request to summarise the talk from before the link.
func (s *Store) AdvanceHistory(ctx context.Context, linkID ids.UUID, from *int64, to int64, backfilled bool) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var moved bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chat_link
			SET history_cursor = $3, history_synced_at = now(),
			    backfill_requested = backfill_requested AND NOT $4
			WHERE id = $1 AND archived_at IS NULL AND history_cursor IS NOT DISTINCT FROM $2`,
			linkID, from, to, backfilled)
		moved = err == nil && tag.RowsAffected() == 1
		return err
	})
	return moved, err
}

// FilingLink reads the live link of a record a member files a channel message
// on. The record must be visible to the caller; an unknown or hidden one
// answers ErrNotFound, and one with no channel ErrLinkNotFound.
func (s *Store) FilingLink(ctx context.Context, kind string, id ids.UUID) (Link, error) {
	if _, err := recordColumn(kind); err != nil {
		return Link{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return Link{}, err
	}
	var out Link
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
			return err
		}
		var err error
		out, err = liveLinkOf(ctx, tx, kind, id)
		return err
	})
	return out, err
}
