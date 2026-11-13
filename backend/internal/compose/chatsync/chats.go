// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package chatsync is the engine of the chat channel links. It acts as the
// connected bot. It turns events into pending items of a channel, and sends
// them as posts with a card kept current.
package chatsync

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// callTimeout bounds one call to the chat, so a hung install cannot hold
// a request or a worker.
const callTimeout = 15 * time.Second

// chatHTTPClient builds the client the bot calls the chat with. A team hosts
// Dispact itself, so in production its address must resolve to a public IP.
// Otherwise a token's owner could aim this server at its own network. Dev and
// test run the chat on loopback.
func HTTPClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: callTimeout}
	if !allowPrivate {
		dialer.Control = netguard.RefusePrivate
	}
	return &http.Client{
		Timeout:       callTimeout,
		Transport:     &http.Transport{DialContext: dialer.DialContext, ForceAttemptHTTP2: true},
		CheckRedirect: netguard.RefuseRedirect,
	}
}

// LiveChats opens a session as the connected Dispact bot. The vault is read
// when a session opens, because the API learns it from a later option.
type LiveChats struct {
	DB    *database.DB
	Vault func() keyvault.Vault
	HTTP  dispact.HTTPDoer
}

func (c LiveChats) Open(ctx context.Context) (chatlinks.Session, error) {
	vault := c.Vault()
	if vault == nil {
		return chatlinks.Session{}, chatlinks.ErrBotNotConnected
	}
	registry := capture.NewRegistry(c.DB, nil, nil, vault)
	bot, err := registry.LiveBot(ctx, chatlinks.ProviderDispact)
	if errors.Is(err, capture.ErrNoConnection) {
		return chatlinks.Session{}, chatlinks.ErrBotNotConnected
	}
	if err != nil {
		return chatlinks.Session{}, err
	}
	return chatlinks.Session{
		ConnectionID: bot.ConnectionID,
		TeamID:       bot.TeamID,
		Address:      bot.Address,
		Chat:         dispact.New(c.HTTP, bot.Address, bot.Token),
	}, nil
}

// BotCheck confirms a Dispact app token against the install's address.
// The team id becomes the connection's channel id, which is what a call
// token's workspace_id claim is checked against.
type BotCheck struct {
	HTTP dispact.HTTPDoer
	// AllowHTTP admits a plain http address, which only dev and test allow:
	// the token rides every request.
	AllowHTTP bool
}

func (BotCheck) NeedsAddress() bool { return true }

func (BotCheck) ValidateToken(token string) error {
	if !strings.HasPrefix(token, dispact.TokenPrefix) || len(token) <= len(dispact.TokenPrefix) {
		return fmt.Errorf("a Dispact app token starts with %s: %w", dispact.TokenPrefix, capture.ErrChannelTokenRejected)
	}
	return nil
}

func (BotCheck) FixedIdentity() bool { return true }

func (c BotCheck) Confirm(ctx context.Context, address, token string) (capture.ChannelBot, error) {
	if !c.AllowHTTP && !strings.HasPrefix(address, "https://") {
		return capture.ChannelBot{}, fmt.Errorf("the chat address must start with https://: %w", apperrors.ErrInvalidArgument)
	}
	bot, err := dispact.New(c.HTTP, address, token).Me(ctx)
	switch {
	case errors.Is(err, dispact.ErrTokenRejected), errors.Is(err, dispact.ErrNotMember):
		return capture.ChannelBot{}, fmt.Errorf("%w: %w", capture.ErrChannelTokenRejected, err)
	case err != nil:
		return capture.ChannelBot{}, fmt.Errorf("%w: %w", capture.ErrChannelProviderUnreachable, err)
	case bot.TeamID == "":
		return capture.ChannelBot{}, fmt.Errorf("the chat named no team for this bot: %w", capture.ErrChannelProviderUnreachable)
	}
	label := bot.Name
	if label == "" {
		label = bot.ID
	}
	return capture.ChannelBot{ID: bot.TeamID, Label: label}, nil
}

// Wire builds the chat links store and the session opener over db, with the
// flush scheduled through enqueuer.
func Wire(db *database.DB, vault func() keyvault.Vault, enqueuer Enqueuer, allowPrivate bool) (*chatlinks.Store, LiveChats) {
	chats := LiveChats{DB: db, Vault: vault, HTTP: HTTPClient(allowPrivate)}
	return chatlinks.NewStore(db, chats, enqueuer), chats
}
