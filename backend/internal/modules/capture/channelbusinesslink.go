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
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
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

// chatLinkProviders are the providers whose approved account rows file a stored
// message through a seat.
const chatLinkProviders = `('` + telegram.BusinessProvider + `', '` + whatsapp.Provider + `')`

// LockLiveChatLinkTx reads the account and ownership of the connection a stored
// message was admitted through. It holds the row FOR SHARE until the caller's
// transaction ends. Every writer that disconnects, disables or archives the row
// updates it and waits on that lock. It answers false for a connection that is
// archived, switched off or has no seat. Only `connected` and `reauth_required`
// are live: the second still receives and a reconnect repairs it. A connection
// switched off after admission must not file what it queued.
func LockLiveChatLinkTx(ctx context.Context, tx pgx.Tx, linkID ids.UUID) (connector.ChatLink, bool, error) {
	_, link, err := scanBusinessLink(tx.QueryRow(ctx, `
		SELECT `+businessLinkColumns+` FROM channel_connection
		 WHERE id = $1 AND provider IN `+chatLinkProviders+`
		   AND status IN ($2, $3) AND seat_id IS NOT NULL AND archived_at IS NULL FOR SHARE`,
		linkID, channelStatusConnected, channelStatusReauthRequired))
	if errors.Is(err, pgx.ErrNoRows) {
		return connector.ChatLink{}, false, nil
	}
	if err != nil {
		return connector.ChatLink{}, false, fmt.Errorf("capture: reading the chat connection of a stored message: %w", err)
	}
	return link, true, nil
}

const businessLinkColumns = `id, account_id, ownership, owner_user_id, seat_id`

func scanBusinessLink(row pgx.Row) (ids.UUID, connector.ChatLink, error) {
	var id ids.UUID
	var link connector.ChatLink
	var ownership string
	var owner, seat *ids.UUID
	if err := row.Scan(&id, &link.AccountID, &ownership, &owner, &seat); err != nil {
		return ids.Nil, connector.ChatLink{}, err
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

// ErrDiscardOriginal is what a file callback returns when the message will not be
// filed for a privacy reason. The stored original is then destroyed in the same
// transaction, so no copy of an erased customer's message outlives the refusal.
var ErrDiscardOriginal = errors.New("capture: the stored original is not filed and must not be kept")

// FileThroughLiveChatLink runs file while the connection a stored message came
// through is still live. It holds the link row for the whole call, so a
// disconnect or disable waits for the filing. A link that is no longer live files
// nothing and the stored original is destroyed. It reports whether file ran.
func FileThroughLiveChatLink(
	ctx context.Context, pool *pgxpool.Pool, linkID string, rawID ids.UUID, file func(connector.ChatLink) error,
) (bool, error) {
	id, err := ids.Parse(linkID)
	if err != nil {
		return false, fmt.Errorf("capture: business link id: %w", err)
	}
	live := false
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		link, found, err := LockLiveChatLinkTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if !found {
			return DeleteRawCaptureTx(ctx, tx, rawID)
		}
		live = true
		if err := file(link); errors.Is(err, ErrDiscardOriginal) {
			return DeleteRawCaptureTx(ctx, tx, rawID)
		} else if err != nil {
			return err
		}
		return nil
	})
	return live, err
}
