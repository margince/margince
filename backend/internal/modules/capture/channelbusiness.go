// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Telegram Business connections. The owner of a Telegram Business account links
// the workspace's bot to their account inside Telegram. The link exists before
// anyone here has seen it. A workspace admin must approve it, and until then
// nothing from that account is stored.
//
// A link is a row of channel_connection that rides on the bot connection receiving
// its updates (parent_connection_id). It holds no credential, so nothing here
// touches the vault. The writes are audit-only for the reason channelconn.go gives.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// channelStatusPendingApproval: the owner linked the bot and no admin has approved it.
const channelStatusPendingApproval = "pending_approval"

// businessConnectionRow selects the Telegram Business links. The provider is named
// so a child row of another provider never reads as a Telegram link.
const businessConnectionRow = "provider = '" + telegram.BusinessProvider + "' AND parent_connection_id IS NOT NULL"

// BusinessConnection is one Telegram Business account link as an admin reads it.
type BusinessConnection struct {
	ID ids.UUID
	// AccountID is the owner's Telegram user id.
	AccountID string
	// OwnerLabel is the owner's name as Telegram last reported it.
	OwnerLabel string
	Status     string
	Version    int64
	// ConnectedAt is when the owner first linked the bot.
	ConnectedAt time.Time
	ApprovedAt  *time.Time
	// SeatID, ReaderTeam and Visibility are the account's seat, the team that
	// reads it and its mail visibility (held or shared, or classified for a
	// member). All nil until an admin approves the account.
	SeatID     *ids.UUID
	ReaderTeam *ids.UUID
	Visibility *string
	// Owner is the member behind a member-owned account, nil for a company's.
	Owner *ids.UUID
}

const businessConnectionColumns = `id, account_id, channel_label, status, version, created_at, approved_at, seat_id,
	(SELECT s.reader_team_id FROM capture_connection s WHERE s.id = channel_connection.seat_id),
	(SELECT s.mail_posture FROM capture_connection s WHERE s.id = channel_connection.seat_id), owner_user_id`

func scanBusinessConnection(r pgx.Row) (BusinessConnection, error) {
	var c BusinessConnection
	err := r.Scan(&c.ID, &c.AccountID, &c.OwnerLabel, &c.Status, &c.Version, &c.ConnectedAt, &c.ApprovedAt,
		&c.SeatID, &c.ReaderTeam, &c.Visibility, &c.Owner)
	return c, err
}

func businessAuditImage(c BusinessConnection, parent ids.UUID) map[string]any {
	return map[string]any{
		detailProvider:         telegram.BusinessProvider,
		"account_id":           c.AccountID,
		"channel_label":        c.OwnerLabel,
		columnStatus:           c.Status,
		"parent_connection_id": parent.String(),
		"approved":             c.ApprovedAt != nil,
	}
}

// businessStatusFor is the status a link takes when Telegram reports it enabled
// or disabled. An approved link returns to `connected` when the owner enables it
// again. An unapproved one goes back to waiting.
func businessStatusFor(bc telegram.BusinessConnection, approved bool) string {
	switch {
	case !bc.Enabled:
		return channelStatusDisconnected
	case approved:
		return channelStatusConnected
	default:
		return channelStatusPendingApproval
	}
}

// recordTelegramBusinessConnection applies one business_connection update to the
// row of its owner's account, in the transaction that advances the poll cursor.
// The row is found by the owner's Telegram user id. A link made again by the same
// owner keeps its row and its approval, and another owner never reaches it.
//
// A disabled link nobody has recorded writes nothing, since there is no account to
// track. A repeated update changes nothing and writes no audit row.
func recordTelegramBusinessConnection(ctx context.Context, tx pgx.Tx, botConnection ids.UUID, bc telegram.BusinessConnection) error {
	var connectedBy ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT connected_by FROM channel_connection
		 WHERE id = $1 AND provider = $2 AND archived_at IS NULL AND `+botConnectionRow+` FOR SHARE`,
		botConnection, telegram.Provider).Scan(&connectedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Disconnected while the batch was in flight; its links went with it.
			return nil
		}
		return fmt.Errorf("capture: reading the bot connection a business connection rides on: %w", err)
	}
	var current BusinessConnection
	var storedID string
	err := tx.QueryRow(ctx, `
		SELECT `+businessConnectionColumns+`, channel_id FROM channel_connection
		 WHERE provider = $1 AND account_id = $2 AND archived_at IS NULL FOR UPDATE`,
		telegram.BusinessProvider, bc.OwnerID).
		Scan(&current.ID, &current.AccountID, &current.OwnerLabel, &current.Status, &current.Version,
			&current.ConnectedAt, &current.ApprovedAt, &current.SeatID, &current.ReaderTeam, &current.Visibility, &current.Owner, &storedID)
	if errors.Is(err, pgx.ErrNoRows) {
		if !bc.Enabled {
			return nil
		}
		return insertBusinessConnection(ctx, tx, botConnection, connectedBy, bc)
	}
	if err != nil {
		return fmt.Errorf("capture: reading the business connection of an account: %w", err)
	}
	want := businessStatusFor(bc, current.ApprovedAt != nil)
	if want == current.Status && storedID == bc.ID && current.OwnerLabel == bc.OwnerLabel {
		return nil
	}
	after := current
	after.Status, after.OwnerLabel = want, bc.OwnerLabel
	tag, err := tx.Exec(ctx, `
		UPDATE channel_connection SET status = $2, channel_id = $3, channel_label = $4, parent_connection_id = $5
		 WHERE id = $1 AND version = $6 AND archived_at IS NULL`, current.ID, want, bc.ID, bc.OwnerLabel, botConnection, current.Version)
	if err != nil {
		return fmt.Errorf("capture: updating the business connection of an account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrVersionSkew
	}
	if err := moveBusinessSeatTx(ctx, tx, current.SeatID, want); err != nil {
		return err
	}
	return auditLifecycle(ctx, tx, "update", channelConnectionObject, current.ID,
		businessAuditImage(current, botConnection), businessAuditImage(after, botConnection))
}

// moveBusinessSeatTx keeps a link's seat as live as the link. The seat ends when
// the owner switches the link off in Telegram. It wakes when they switch it on.
func moveBusinessSeatTx(ctx context.Context, tx pgx.Tx, seat *ids.UUID, linkStatus string) error {
	switch {
	case seat == nil:
		return nil
	case linkStatus == channelStatusConnected:
		return setChatSeatStatusTx(ctx, tx, *seat, channelStatusConnected)
	case linkStatus == channelStatusDisconnected:
		return EndChatSeatTx(ctx, tx, *seat)
	}
	return nil
}

// RecordTelegramBusinessUpdateTx applies one polled update when it is a
// business_connection and leaves other kinds alone. One that cannot be decoded is
// skipped, because failing would hold the cursor behind it for good.
func RecordTelegramBusinessUpdateTx(ctx context.Context, tx pgx.Tx, botConnection ids.UUID, update json.RawMessage) error {
	bc, isBusiness, err := telegram.ParseBusinessConnection(update)
	if !isBusiness {
		return nil
	}
	if err != nil {
		// One the poller's classification already refused and logged. It may
		// not hold the cursor.
		return nil //nolint:nilerr // a malformed update is skipped, never retried
	}
	return recordTelegramBusinessConnection(ctx, tx, botConnection, bc)
}

func insertBusinessConnection(ctx context.Context, tx pgx.Tx, botConnection, connectedBy ids.UUID, bc telegram.BusinessConnection) error {
	created, err := scanBusinessConnection(tx.QueryRow(ctx, `
		INSERT INTO channel_connection
		  (provider, channel_id, channel_label, credential_ref, status, connected_by, account_id, parent_connection_id)
		VALUES ($1, $2, $3, '', $4, $5, $6, $7)
		RETURNING `+businessConnectionColumns,
		telegram.BusinessProvider, bc.ID, bc.OwnerLabel, channelStatusPendingApproval, connectedBy, bc.OwnerID, botConnection))
	if err != nil {
		return fmt.Errorf("capture: recording a new business connection: %w", err)
	}
	return auditLifecycle(ctx, tx, "create", channelConnectionObject, created.ID, nil, businessAuditImage(created, botConnection))
}

// archiveBusinessConnectionsOfTx retires the links that ride on a bot connection
// being disconnected. With the bot gone nothing receives their updates.
func archiveBusinessConnectionsOfTx(ctx context.Context, tx pgx.Tx, bot ids.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT `+businessConnectionColumns+` FROM channel_connection
		 WHERE parent_connection_id = $1 AND archived_at IS NULL AND `+businessConnectionRow+` FOR UPDATE`, bot)
	if err != nil {
		return fmt.Errorf("capture: reading the business connections of a bot: %w", err)
	}
	links, err := pgx.CollectRows(rows, pgx.RowToStructByPos[BusinessConnection])
	if err != nil {
		return fmt.Errorf("capture: reading the business connections of a bot: %w", err)
	}
	for _, link := range links {
		if err := archiveBusinessConnectionTx(ctx, tx, link, bot); err != nil {
			return err
		}
	}
	return nil
}

func archiveBusinessConnectionTx(ctx context.Context, tx pgx.Tx, link BusinessConnection, bot ids.UUID) error {
	after := link
	after.Status = channelStatusDisconnected
	tag, err := tx.Exec(ctx,
		`UPDATE channel_connection SET status = $2, archived_at = now() WHERE id = $1 AND version = $3 AND archived_at IS NULL`,
		link.ID, after.Status, link.Version)
	if err != nil {
		return fmt.Errorf("capture: archiving a business connection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrVersionSkew
	}
	if link.SeatID != nil {
		if err := EndChatSeatTx(ctx, tx, *link.SeatID); err != nil {
			return err
		}
	}
	return auditLifecycle(ctx, tx, "archive", channelConnectionObject, link.ID,
		businessAuditImage(link, bot), businessAuditImage(after, bot))
}

// requireBusinessAdmin gates the approval surface. A link names an owner's account
// and decides whose chats are captured, so reading the list needs the right to
// change it.
func requireBusinessAdmin(ctx context.Context, action principal.Action) error {
	if err := auth.Require(ctx, channelConnectionObject, action); err != nil {
		return err
	}
	return auth.RequireHuman(ctx)
}

// ListBusinessConnections returns the live Telegram Business links of the
// workspace, newest first.
func (s *ChannelStore) ListBusinessConnections(ctx context.Context) ([]BusinessConnection, error) {
	if err := requireBusinessAdmin(ctx, principal.ActionUpdate); err != nil {
		return nil, err
	}
	var out []BusinessConnection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+businessConnectionColumns+` FROM channel_connection
			 WHERE archived_at IS NULL AND `+businessConnectionRow+` ORDER BY created_at DESC, id DESC`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[BusinessConnection])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing business connections: %w", err)
	}
	return out, nil
}

// ApproveBusinessConnection makes a waiting link `connected` and gives its account
// its seat. A company account needs the team that reads it (readerTeam) the first
// time, and starts held. Approving a connected link changes nothing except that
// it makes a seat that is missing. A link the owner switched off cannot be
// approved, because it would read as live while Telegram sends nothing.
func (s *ChannelStore) ApproveBusinessConnection(ctx context.Context, id, readerTeam ids.UUID) (BusinessConnection, error) {
	if err := requireBusinessAdmin(ctx, principal.ActionUpdate); err != nil {
		return BusinessConnection{}, err
	}
	var out BusinessConnection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, parent, err := lockBusinessConnection(ctx, tx, id)
		if err != nil {
			return err
		}
		out = current
		switch current.Status {
		case channelStatusConnected, channelStatusPendingApproval:
		default:
			return fmt.Errorf("the owner has switched this business connection off in Telegram; ask them to turn it on first: %w", apperrors.ErrConflict)
		}
		seat, err := seatBusinessConnectionTx(ctx, tx, current, readerTeam)
		if err != nil {
			return err
		}
		out, err = scanBusinessConnection(tx.QueryRow(ctx, `
			UPDATE channel_connection SET status = $2, approved_at = coalesce(approved_at, now()), seat_id = $4
			 WHERE id = $1 AND version = $3 AND archived_at IS NULL
			RETURNING `+businessConnectionColumns, id, channelStatusConnected, current.Version, seat))
		if err != nil {
			return err
		}
		if current.Status == channelStatusConnected && holdsSeat(current.SeatID, seat) {
			return nil
		}
		return auditLifecycle(ctx, tx, "update", channelConnectionObject, id,
			businessAuditImage(current, parent), businessAuditImage(out, parent))
	})
	if err != nil {
		return BusinessConnection{}, businessErr("approving", err)
	}
	return out, nil
}

func holdsSeat(a *ids.UUID, b ids.UUID) bool { return a != nil && *a == b }

// seatBusinessConnectionTx finds or makes the seat of an approved account.
func seatBusinessConnectionTx(ctx context.Context, tx pgx.Tx, link BusinessConnection, readerTeam ids.UUID) (ids.UUID, error) {
	actor, _ := principal.Actor(ctx)
	seat := ChatSeat{Provider: telegram.BusinessProvider, Account: link.AccountID, ReaderTeam: readerTeam, By: actor.UserID}
	if link.Owner != nil {
		seat.Owner = *link.Owner
	}
	return EnsureChatSeatTx(ctx, tx, seat)
}

// DisconnectBusinessConnection withdraws the workspace's acceptance of a link and
// archives its row. Nothing is destroyed and what the account's chats already
// filed stays. If the owner links the bot again, the new link waits for approval.
func (s *ChannelStore) DisconnectBusinessConnection(ctx context.Context, id ids.UUID) error {
	if err := requireBusinessAdmin(ctx, principal.ActionDelete); err != nil {
		return err
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, parent, err := lockBusinessConnection(ctx, tx, id)
		if err != nil {
			return err
		}
		return archiveBusinessConnectionTx(ctx, tx, current, parent)
	})
	return businessErr("disconnecting", err)
}

// lockBusinessConnection locks one live link and answers with its bot connection.
// Any other row, including a bot connection, reads as ErrNotFound.
func lockBusinessConnection(ctx context.Context, tx pgx.Tx, id ids.UUID) (BusinessConnection, ids.UUID, error) {
	var current BusinessConnection
	var parent ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT `+businessConnectionColumns+`, parent_connection_id FROM channel_connection
		 WHERE id = $1 AND archived_at IS NULL AND `+businessConnectionRow+` FOR UPDATE`, id).
		Scan(&current.ID, &current.AccountID, &current.OwnerLabel, &current.Status, &current.Version,
			&current.ConnectedAt, &current.ApprovedAt, &current.SeatID, &current.ReaderTeam, &current.Visibility, &current.Owner, &parent)
	if errors.Is(err, pgx.ErrNoRows) {
		return BusinessConnection{}, ids.Nil, apperrors.ErrNotFound
	}
	return current, parent, err
}

func businessErr(doing string, err error) error {
	if err == nil || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
		return err
	}
	return fmt.Errorf("capture: %s a business connection: %w", doing, err)
}
