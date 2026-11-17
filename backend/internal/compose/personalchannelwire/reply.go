// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannelwire

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ErrNotTheirChannel refuses a reply from anyone but the member whose chat app
// carries the thread. It would go out as words that member did not write. It
// parks at once: no retry makes it someone else's chat app.
var ErrNotTheirChannel = fmt.Errorf("only the member whose personal channel brought this chat in can reply here, from their own chat app: %w", connector.ErrRecipientUnreachable)

// ReplySender queues a reply for the client of the member who sends it.
type ReplySender struct {
	DB     *database.DB
	Member ids.UUID
}

var _ connector.MessageSender = ReplySender{}

// SendMessage queues msg on the connection that brought its thread in. The core
// counts it sent once queued; the client's ack, or the 7-day expiry, may later
// mark it not delivered.
func (s ReplySender) SendMessage(ctx context.Context, _ connector.Auth, msg connector.ChannelMessage) (connector.SendReceipt, error) {
	if len(msg.Files) > 0 {
		return connector.SendReceipt{}, connector.ErrFilesNotCarried
	}
	network, account, ok := personalchannels.SplitIdentity(msg.Recipient.ChannelUserID)
	if !ok {
		return connector.SendReceipt{}, fmt.Errorf("personal channels: recipient %q names no network: %w", msg.Recipient.ChannelUserID, comms.ErrCannotSend)
	}
	err := s.DB.Tx(ctx, func(tx pgx.Tx) error {
		connection, group, err := s.threadConnection(ctx, tx, msg.ActivityID.UUID)
		if err != nil {
			return err
		}
		if personalchannels.IdentityConnection(msg.Recipient.ChannelUserID) != connection.String() {
			return fmt.Errorf("personal channels: the recipient was met through another member's chat app: %w", connector.ErrRecipientUnreachable)
		}
		return personalchannels.QueueReply(ctx, tx, personalchannels.ReplyInput{
			Connection: connection, ActivityID: msg.ActivityID.UUID, DeliveryKey: msg.IdempotencyKey,
			Network: network, Account: account, GroupID: group, ReplyTo: msg.ReplyTo, Body: msg.Body,
		})
	})
	if errors.Is(err, personalchannels.ErrReplyClosed) {
		err = fmt.Errorf("%w: %w", err, connector.ErrRecipientUnreachable)
	}
	return connector.SendReceipt{}, err
}

// threadConnection is the connection whose thread the reply joins, read from the
// reply's thread key (personal_channels:<connection>:<identity>[:<group>]), when
// it belongs to the sender and is on.
func (s ReplySender) threadConnection(ctx context.Context, tx pgx.Tx, activity ids.UUID) (ids.UUID, string, error) {
	var threadKey string
	if err := tx.QueryRow(ctx, `SELECT coalesce(thread_key, '') FROM activity WHERE id = $1`, activity).Scan(&threadKey); err != nil {
		return ids.UUID{}, "", err
	}
	parts := strings.SplitN(threadKey, ":", 4)
	if len(parts) < 3 || parts[0] != personalchannels.Provider {
		return ids.UUID{}, "", fmt.Errorf("personal channels: the reply joins no personal channel thread: %w", comms.ErrCannotSend)
	}
	connection, err := ids.Parse(parts[1])
	if err != nil {
		return ids.UUID{}, "", fmt.Errorf("personal channels: the thread names no connection: %w", comms.ErrCannotSend)
	}
	var owner ids.UUID
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT user_id, enabled FROM personal_channel_connection WHERE id = $1`, connection).Scan(&owner, &enabled)
	switch {
	case errors.Is(err, pgx.ErrNoRows), err == nil && owner != s.Member:
		return ids.UUID{}, "", ErrNotTheirChannel
	case err != nil:
		return ids.UUID{}, "", err
	case !enabled:
		return ids.UUID{}, "", fmt.Errorf("personal channels are off for this member: %w", connector.ErrRecipientUnreachable)
	}
	group := ""
	if len(parts) == 4 {
		group = parts[3]
	}
	return connection, group, nil
}

// SendCapable reports whether member's personal channels are on, so a reply
// is refused when it is written rather than parked after.
func SendCapable(ctx context.Context, db *database.DB, member ids.UUID) (bool, error) {
	var on bool
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT enabled FROM personal_channel_connection WHERE user_id = $1`, member).Scan(&on)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return on, err
}

// OwnIdentities keeps the identities the member's own chat app met. A colleague's
// thread then reaches nobody for this member, so a reply is refused when it is
// written, not parked after.
func OwnIdentities(ctx context.Context, tx pgx.Tx, member ids.UUID, identities []connector.ChannelIdentity) ([]connector.ChannelIdentity, error) {
	var connection ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM personal_channel_connection WHERE user_id = $1`, member).Scan(&connection)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var own []connector.ChannelIdentity
	for _, identity := range identities {
		if personalchannels.IdentityConnection(identity.ChannelUserID) == connection.String() {
			own = append(own, identity)
		}
	}
	return own, nil
}

// Settler marks a reply not delivered on its comms delivery.
type Settler struct{}

var _ personalchannels.Settler = Settler{}

// NotDelivered settles the reply's delivery as not delivered.
func (Settler) NotDelivered(ctx context.Context, tx pgx.Tx, activity ids.UUID, reason string) error {
	return comms.RecordChannelNotDeliveredTx(ctx, tx, time.Now(), activity, reason)
}
