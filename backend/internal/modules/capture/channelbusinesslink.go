// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The two reads a Telegram Business message needs from its link row. The first
// says whether the message may be stored at all, and the poller asks it before
// it persists anything. The second says whose account and ownership a stored
// message files under.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// AdmitTelegramUpdateTx is the poller's admission check for an update it is about
// to store. A message to the bot itself answers ("", true).
//
// A business_message answers true with its link's row id only when it names an
// approved link of this bot. Otherwise the caller stores nothing, not even the
// update, for an account no admin has accepted.
func AdmitTelegramUpdateTx(ctx context.Context, tx pgx.Tx, bot ids.UUID, update []byte) (string, bool, error) {
	connectionID, isBusiness, err := telegram.BusinessConnectionIDOf(update)
	if err != nil {
		return "", false, err
	}
	if !isBusiness {
		return "", true, nil
	}
	if connectionID == "" {
		return "", false, nil
	}
	linkID, connected, err := connectedBusinessLinkTx(ctx, tx, bot, connectionID)
	if err != nil || !connected {
		return "", false, err
	}
	return linkID.String(), true, nil
}

// connectedBusinessLinkTx resolves the connection id a business_message names to
// the approved link of this bot. It answers false for an unknown id, a link
// waiting for an admin, and a link switched off.
func connectedBusinessLinkTx(ctx context.Context, tx pgx.Tx, bot ids.UUID, businessConnectionID string) (ids.UUID, bool, error) {
	id, _, err := scanBusinessLink(tx.QueryRow(ctx, `
		SELECT `+businessLinkColumns+` FROM channel_connection
		 WHERE provider = $1 AND channel_id = $2 AND parent_connection_id = $3
		   AND status = $4 AND archived_at IS NULL`,
		telegram.BusinessProvider, businessConnectionID, bot, channelStatusConnected))
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("capture: resolving the business link of a message: %w", err)
	}
	return id, true, nil
}

// BusinessLinkTx reads the account and ownership of one link by its row id, for
// the worker that files a message the poller admitted. It skips the status check.
// Admission was decided on arrival, and a link switched off since must not strand
// a stored message.
func BusinessLinkTx(ctx context.Context, tx pgx.Tx, linkID ids.UUID) (telegram.BusinessLink, error) {
	_, link, err := scanBusinessLink(tx.QueryRow(ctx, `
		SELECT `+businessLinkColumns+` FROM channel_connection
		 WHERE id = $1 AND provider = $2 AND parent_connection_id IS NOT NULL`, linkID, telegram.BusinessProvider))
	if err != nil {
		return telegram.BusinessLink{}, fmt.Errorf("capture: reading the business link of a stored message: %w", err)
	}
	return link, nil
}

const businessLinkColumns = `id, account_id, ownership, owner_user_id`

func scanBusinessLink(row pgx.Row) (ids.UUID, telegram.BusinessLink, error) {
	var id ids.UUID
	var link telegram.BusinessLink
	var ownership string
	var owner *ids.UUID
	if err := row.Scan(&id, &link.AccountID, &ownership, &owner); err != nil {
		return ids.Nil, telegram.BusinessLink{}, err
	}
	link.Ownership = connector.ChatOwnership(ownership)
	if owner != nil {
		link.OwnerUserID = *owner
	}
	return id, link, nil
}

// BusinessLinkOfJob reads the link an ingest job was admitted through. The caller
// asks only for a job that carries a link id: a job whose update was sent to the
// bot itself has none.
func BusinessLinkOfJob(ctx context.Context, pool *pgxpool.Pool, linkID string) (telegram.BusinessLink, error) {
	id, err := ids.Parse(linkID)
	if err != nil {
		return telegram.BusinessLink{}, fmt.Errorf("capture: business link id: %w", err)
	}
	var link telegram.BusinessLink
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		link, err = BusinessLinkTx(ctx, tx, id)
		return err
	})
	return link, err
}
