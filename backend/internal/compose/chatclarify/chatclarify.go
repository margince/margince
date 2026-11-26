// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package chatclarify runs one workspace's pass over the messages colleagues
// forwarded to a company chat account without naming the author. It forms the
// batches, asks each colleague who wrote them, and sends the follow-ups. The
// accounts send nothing else. What is sent, and to whom, is decided in the
// capture module; this package supplies the networks' senders and the clock.
package chatclarify

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatmembers"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/telegram"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// actorID names the sweep in whatever it touches.
const actorID = "system:chat-clarification"

// Sweep is the pass's dependencies. A nil client takes the network's own host.
type Sweep struct {
	Pool  *pgxpool.Pool
	Vault keyvault.Vault
	// Clients are the networks' API clients.
	WhatsApp *whatsapp.Client
	Zalo     *zalo.Client
	Telegram telegram.API
	// Bot resolves the workspace's own Telegram bot. Nil in a role that holds no
	// capture registry, where a colleague who writes to the bot is not asked.
	Bot func(ctx context.Context) (connector.MessageSender, connector.Auth, error)
	Now func() time.Time
	Log *slog.Logger
}

// Workspace runs the pass for one tenant. No member decides it, so it runs as
// the system and is bound to the workspace it was queued for.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), actorID)
	return s.asker(workspace).Sweep(ctx)
}

func (s Sweep) asker(workspace ids.UUID) *capture.ChatAsker {
	db := database.BindTo(s.Pool, ids.From[ids.WorkspaceKind](workspace))
	whatsappClient, zaloClient := s.WhatsApp, s.Zalo
	if whatsappClient == nil {
		whatsappClient = whatsapp.NewClient(nil, "", "")
	}
	if zaloClient == nil {
		zaloClient = zalo.NewClient(nil, "", "")
	}
	return &capture.ChatAsker{
		DB:      db,
		Members: chatmembers.New(s.Pool),
		Senders: capture.ChatSenders{
			DB: db, Vault: s.Vault,
			WhatsApp:         whatsapp.NewSender(whatsappClient),
			Zalo:             zalo.NewSender(zaloClient),
			TelegramBusiness: telegram.New(s.telegram()),
			Bot:              s.Bot,
		},
		Now: s.Now,
		Log: s.Log,
	}
}

//nolint:ireturn // the Bot API client is the capture/telegram package's own interface
func (s Sweep) telegram() telegram.API {
	if s.Telegram == nil {
		return telegram.NewAPI(nil, "")
	}
	return s.Telegram
}

// BotOf resolves the workspace's own Telegram bot through the capture registry,
// or answers nil where a role holds none.
func BotOf(registry *capture.Registry) func(ctx context.Context) (connector.MessageSender, connector.Auth, error) {
	if registry == nil {
		return nil
	}
	return func(ctx context.Context) (connector.MessageSender, connector.Auth, error) {
		return registry.ChannelSenderFor(ctx, telegram.Provider)
	}
}
