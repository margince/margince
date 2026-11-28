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
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// callTimeout bounds one call to the chat, so a hung install cannot hold
// a request or a worker.
const callTimeout = 15 * time.Second

// HTTPClient builds the client the bot calls the chat with. A team hosts
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
	TeamThreads *chatlinks.Store
	App         BotApp
	Now         func() time.Time
	DB          *database.DB
	Vault       func() keyvault.Vault
	HTTP        dispact.HTTPDoer
}

// Open implements chatlinks.Chats.
func (c LiveChats) Open(ctx context.Context) (chatlinks.Session, error) {
	return c.OpenProvider(ctx, chatlinks.ProviderDispact)
}

// OpenProvider opens the selected installation instead of guessing another provider.
func (c LiveChats) OpenProvider(ctx context.Context, provider string) (chatlinks.Session, error) {
	vault := c.Vault()
	if vault == nil {
		return chatlinks.Session{}, chatlinks.ErrBotNotConnected
	}
	registry := capture.NewRegistry(c.DB, nil, nil, vault)
	bot, err := registry.LiveBot(ctx, provider)
	if errors.Is(err, capture.ErrNoConnection) {
		return chatlinks.Session{}, chatlinks.ErrBotNotConnected
	}
	if err != nil {
		return chatlinks.Session{}, err
	}
	var chat chatlinks.Chat
	switch provider {
	case chatlinks.ProviderDispact:
		client := dispact.New(c.HTTP, bot.Address, bot.Token)
		identity, err := client.Me(ctx)
		if err != nil {
			return chatlinks.Session{}, err
		}
		if identity.TeamID != bot.TeamID {
			return chatlinks.Session{}, chatwire.ErrTokenRejected
		}
		chat = client
		if dispact.CanonicalAvailable(identity) {
			chat = dispact.NewCanonical(client)
		}
	case chatlinks.ProviderSlack:
		chat, err = c.openSlack(ctx, registry)
		if err != nil {
			return chatlinks.Session{}, err
		}
	case chatlinks.ProviderTeams:
		chat, bot.TeamID, err = c.openTeams(ctx, bot)
		if err != nil {
			return chatlinks.Session{}, err
		}
	default:
		return chatlinks.Session{}, chatlinks.ErrBotNotConnected
	}
	return chatlinks.Session{
		Provider:     provider,
		ConnectionID: bot.ConnectionID,
		TeamID:       bot.TeamID,
		Address:      bot.Address,
		Chat:         chat,
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

// NeedsAddress is true: every team hosts its own Dispact.
func (BotCheck) NeedsAddress() bool { return true }

// ValidateToken refuses a token without the Dispact app prefix.
func (BotCheck) ValidateToken(token string) error {
	if !strings.HasPrefix(token, dispact.TokenPrefix) || len(token) <= len(dispact.TokenPrefix) {
		return fmt.Errorf("a Dispact app token starts with %s: %w", dispact.TokenPrefix, capture.ErrChannelTokenRejected)
	}
	return nil
}

// FixedIdentity is true: links are bound to the bot's team.
func (BotCheck) FixedIdentity() bool { return true }

// ValidateEventsSecret makes Dispact a provider that pushes signed notices, so
// its connection takes an events secret.
func (BotCheck) ValidateEventsSecret(secret string) error { return webhooks.CheckSecret(secret) }

// Confirm asks the chat who the token belongs to.
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
func Wire(db *database.DB, vault func() keyvault.Vault, enqueuer Enqueuer, allowPrivate bool) (*chatlinks.Store, *LiveChats) {
	chats := &LiveChats{DB: db, Vault: vault, HTTP: HTTPClient(allowPrivate)}
	store := chatlinks.NewStore(db, chats, enqueuer, Viewers).WithVault(vault)
	chats.TeamThreads = store
	return store, chats
}
