// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The two reads a Telegram Business message needs from its link row. The first
// says whether the message may be stored at all, and the poller asks it before
// it persists anything. The second says whether the link still carries it, and
// whose account and ownership a stored message files under.

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
// waiting for an admin and a link switched off. It also answers false for a
// connected link with no seat, because nothing could file its messages.
func connectedBusinessLinkTx(ctx context.Context, tx pgx.Tx, bot ids.UUID, businessConnectionID string) (ids.UUID, bool, error) {
	id, _, err := scanBusinessLink(tx.QueryRow(ctx, `
		SELECT `+businessLinkColumns+` FROM channel_connection
		 WHERE provider = $1 AND channel_id = $2 AND parent_connection_id = $3
		   AND status = $4 AND seat_id IS NOT NULL AND archived_at IS NULL`,
		telegram.BusinessProvider, businessConnectionID, bot, channelStatusConnected))
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("capture: resolving the business link of a message: %w", err)
	}
	return id, true, nil
}

// LockLiveBusinessLinkTx reads the account and ownership of the link a stored
// message was admitted through. It holds the row FOR SHARE until the caller's
// transaction ends. Every writer that disconnects, disables or archives the link
// updates the row and waits on that lock. It answers false for a link that is no
// longer connected, because a link switched off after admission must not file
// what it queued.
func LockLiveBusinessLinkTx(ctx context.Context, tx pgx.Tx, linkID ids.UUID) (telegram.BusinessLink, bool, error) {
	_, link, err := scanBusinessLink(tx.QueryRow(ctx, `
		SELECT `+businessLinkColumns+` FROM channel_connection
		 WHERE id = $1 AND provider = $2 AND parent_connection_id IS NOT NULL
		   AND status = $3 AND seat_id IS NOT NULL AND archived_at IS NULL FOR SHARE`,
		linkID, telegram.BusinessProvider, channelStatusConnected))
	if errors.Is(err, pgx.ErrNoRows) {
		return telegram.BusinessLink{}, false, nil
	}
	if err != nil {
		return telegram.BusinessLink{}, false, fmt.Errorf("capture: reading the business link of a stored message: %w", err)
	}
	return link, true, nil
}

const businessLinkColumns = `id, account_id, ownership, owner_user_id, seat_id`

func scanBusinessLink(row pgx.Row) (ids.UUID, telegram.BusinessLink, error) {
	var id ids.UUID
	var link telegram.BusinessLink
	var ownership string
	var owner, seat *ids.UUID
	if err := row.Scan(&id, &link.AccountID, &ownership, &owner, &seat); err != nil {
		return ids.Nil, telegram.BusinessLink{}, err
	}
	link.Ownership = connector.ChatOwnership(ownership)
	if owner != nil {
		link.OwnerUserID = *owner
	}
	if seat != nil {
		link.SeatID = *seat
	}
	return id, link, nil
}

// FileThroughLiveBusinessLink runs file while the link a stored message came
// through is still connected. It holds the link row for the whole call, so a
// disconnect or disable waits for the filing. A link that is no longer live files
// nothing and the stored original is destroyed. It reports whether file ran.
func FileThroughLiveBusinessLink(
	ctx context.Context, pool *pgxpool.Pool, linkID string, rawID ids.UUID, file func(telegram.BusinessLink) error,
) (bool, error) {
	id, err := ids.Parse(linkID)
	if err != nil {
		return false, fmt.Errorf("capture: business link id: %w", err)
	}
	live := false
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		link, found, err := LockLiveBusinessLinkTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if !found {
			return DeleteRawCaptureTx(ctx, tx, rawID)
		}
		live = true
		return file(link)
	})
	return live, err
}
