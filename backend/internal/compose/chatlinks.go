// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ChatLinkFlushArgs posts what one chat link has pending and refreshes its card.
type ChatLinkFlushArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	LinkID    ids.UUID `json:"link_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ChatLinkFlushArgs) Kind() string { return "chat_link_flush" }

// WorkspaceID binds the flush to its tenant (jobs.WorkspaceScoped).
func (a ChatLinkFlushArgs) WorkspaceID() ids.UUID { return a.Workspace }

// Link names the link the flush posts for.
func (a ChatLinkFlushArgs) Link() ids.UUID { return a.LinkID }

func chatLinks(pool *pgxpool.Pool, vault func() keyvault.Vault, in chatsync.TxInserter, delay time.Duration, private bool) (*chatlinks.Store, chatsync.LiveChats) {
	return chatsync.Wire(InstallationDB(pool), vault, chatsync.Enqueuer{Inserter: in, Delay: delay,
		MaxAttempts: oneOffJobMaxAttempts, States: activeSweepStates,
		Job: func(ws, link ids.UUID) river.JobArgs { return ChatLinkFlushArgs{Workspace: ws, LinkID: link} }}, private)
}

// WithChatLinks composes /channel-links and the Dispact bot connect; it must
// follow WithChannelSurface. A new link's card is scheduled with no window.
func WithChatLinks(inserter *jobs.Runner, private bool) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		store, _ := chatLinks(pool, func() keyvault.Vault { return s.vault }, inserter, 0, private)
		s.chatLinkHandlers = chatlinks.NewHandlers(store)
		s.channelHandlers = s.channelHandlers.WithBotCheck(chatlinks.ProviderDispact, chatsync.BotCheck{HTTP: chatsync.HTTPClient(private), AllowHTTP: private})
	}
}

// NewChatLinkSync builds the cg:chat-links consumer for a worker role.
func NewChatLinkSync(pool *pgxpool.Pool, vault keyvault.Vault, inserter *jobs.Runner, private bool) *chatsync.Sync {
	store, _ := chatLinks(pool, func() keyvault.Vault { return vault }, inserter, chatsync.Window, private)
	return chatsync.New(pool, InstallationDB(pool), store)
}

func registerChatLinkFlush(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	if cfg.ChannelVault != nil {
		store, chats := chatLinks(pool, func() keyvault.Vault { return cfg.ChannelVault }, chatsync.AmbientInserter{},
			chatsync.Window, cfg.SendOrigin.Environment.IsNonProduction())
		addDeclaredWorker[ChatLinkFlushArgs](reg, &chatsync.Worker[ChatLinkFlushArgs]{Bind: workspaceJobCtx,
			Flusher: chatsync.NewFlusher(pool, chats, store, cfg.SendOrigin.PublicBaseURL, log)})
	}
	return nil
}
