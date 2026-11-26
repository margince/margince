// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The sender and credential of one company chat account, for the question the
// account asks a colleague. A sender is the network's connector.MessageSender,
// and the credential is what the account was connected with. The question is the
// only message these accounts send. It goes to a verified colleague, so nothing
// is ever sent into a customer's conversation from here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ChatSenders resolves what a send from one connected account needs.
type ChatSenders struct {
	DB    *database.DB
	Vault keyvault.Vault
	// WhatsApp, Zalo and TelegramBusiness are the networks' own senders.
	WhatsApp, Zalo, TelegramBusiness connector.MessageSender
	// Bot resolves the workspace's own Telegram bot, which a colleague writes to
	// directly. It is the same resolve a reply to a customer's bot chat uses.
	Bot func(ctx context.Context) (connector.MessageSender, connector.Auth, error)
}

// sendAccount is the connection row a send resolves from.
type sendAccount struct {
	provider, account, channel, credential string
	parent                                 *ids.UUID
}

// For answers the sender of the account a colleague's forward arrived through,
// and the credential it sends with. A zero connection is the workspace's own
// Telegram bot.
//
//nolint:ireturn // returns the network's MessageSender, the optional seam a connector implements
func (c ChatSenders) For(ctx context.Context, provider string, connection ids.UUID) (connector.MessageSender, connector.Auth, error) {
	if connection.IsZero() {
		if provider != telegram.Provider || c.Bot == nil {
			return nil, nil, fmt.Errorf("capture: no chat account to send from: %w", ErrNoConnection)
		}
		return c.Bot(ctx)
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok || c.Vault == nil {
		return nil, nil, ErrChannelWiringIncomplete
	}
	var sender connector.MessageSender
	var auth connector.Auth
	err := c.DB.Tx(ctx, func(tx pgx.Tx) error {
		acct, err := liveChatAccountTx(ctx, tx, connection)
		if err != nil {
			return err
		}
		read := func(ref string) (string, error) {
			sealed, err := c.Vault.GetOn(ctx, tx, ids.From[ids.WorkspaceKind](ws), keyvault.Ref(ref))
			return string(sealed), err
		}
		sender, auth, err = c.senderOf(ctx, tx, acct, read)
		return err
	})
	return sender, auth, err
}

// liveChatAccountTx reads a connected account.
func liveChatAccountTx(ctx context.Context, tx pgx.Tx, connection ids.UUID) (sendAccount, error) {
	var acct sendAccount
	err := tx.QueryRow(ctx, `SELECT provider, coalesce(account_id, ''), channel_id, credential_ref, parent_connection_id
		  FROM channel_connection WHERE id = $1 AND status = $2 AND archived_at IS NULL`, connection, channelStatusConnected).
		Scan(&acct.provider, &acct.account, &acct.channel, &acct.credential, &acct.parent)
	if errors.Is(err, pgx.ErrNoRows) {
		return sendAccount{}, fmt.Errorf("capture: the chat account is not connected: %w", ErrNoConnection)
	}
	if err != nil {
		return sendAccount{}, fmt.Errorf("capture: reading a chat account to send from: %w", err)
	}
	return acct, nil
}

//nolint:ireturn // see For
func (c ChatSenders) senderOf(ctx context.Context, tx pgx.Tx, acct sendAccount, read func(ref string) (string, error)) (connector.MessageSender, connector.Auth, error) {
	switch acct.provider {
	case whatsapp.Provider:
		token, err := read(acct.credential)
		return c.WhatsApp, connector.ChatSendAuth{Token: token, Account: acct.account}.Auth(), err
	case zalo.Provider:
		sealed, err := read(acct.credential)
		if err != nil {
			return nil, nil, err
		}
		var tokens zaloTokens
		if err := json.Unmarshal([]byte(sealed), &tokens); err != nil {
			return nil, nil, fmt.Errorf("capture: the Zalo access token cannot be read: %w", err)
		}
		return c.Zalo, connector.ChatSendAuth{Token: tokens.AccessToken}.Auth(), nil
	case telegram.BusinessProvider:
		return c.telegramBusiness(ctx, tx, acct, read)
	}
	return nil, nil, fmt.Errorf("capture: %s accounts send no questions: %w", acct.provider, ErrConnectorCannotSend)
}

// telegramBusiness sends through the bot the account's link rides on, as the
// business account that link names.
//
//nolint:ireturn // see senderOf
func (c ChatSenders) telegramBusiness(ctx context.Context, tx pgx.Tx, acct sendAccount, read func(ref string) (string, error)) (connector.MessageSender, connector.Auth, error) {
	if acct.parent == nil {
		return nil, nil, fmt.Errorf("capture: a Telegram Business link rides on no bot: %w", ErrNoConnection)
	}
	bot, err := liveChatAccountTx(ctx, tx, *acct.parent)
	if err != nil {
		return nil, nil, err
	}
	token, err := read(bot.credential)
	return c.TelegramBusiness, connector.ChatSendAuth{Token: token, Account: acct.channel}.Auth(), err
}
