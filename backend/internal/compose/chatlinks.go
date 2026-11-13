// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/compose/dispactapp"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
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

// ChatPullSweepArgs enqueues one chat_pull per live Dispact bot.
type ChatPullSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ChatPullSweepArgs) Kind() string { return "chat_pull_sweep" }

// FleetWide marks this a dispatcher (jobs.FleetWide).
func (ChatPullSweepArgs) FleetWide() {}

// ChatPullArgs reads the linked channels of one Dispact bot.
type ChatPullArgs struct {
	Workspace    ids.UUID `json:"workspace_id"`
	ConnectionID ids.UUID `json:"connection_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ChatPullArgs) Kind() string { return "chat_pull" }

// WorkspaceID binds the pull to its tenant (jobs.WorkspaceScoped).
func (a ChatPullArgs) WorkspaceID() ids.UUID { return a.Workspace }

// Connection names the bot whose channels the pull reads.
func (a ChatPullArgs) Connection() ids.UUID { return a.ConnectionID }

// InsertOpts makes one pull per bot wait or run at a time, whoever enqueues
// it, because two pulls would judge the same talk.
func (ChatPullArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: sweptJobMaxAttempts,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}}
}

func chatLinks(pool *pgxpool.Pool, vault func() keyvault.Vault, in chatsync.TxInserter, delay time.Duration, private bool) (*chatlinks.Store, chatsync.LiveChats) {
	return chatsync.Wire(InstallationDB(pool), vault, chatsync.Enqueuer{Inserter: in, Delay: delay,
		MaxAttempts: oneOffJobMaxAttempts, States: activeSweepStates,
		Job: func(ws, link ids.UUID) river.JobArgs { return ChatLinkFlushArgs{Workspace: ws, LinkID: link} }}, private)
}

// WithChatLinks composes /channel-links and the Dispact bot connect; it must
// follow WithChannelSurface. A new link's card is scheduled with no window.
// lane is the chat_summary lane a chosen message is summarised on.
func WithChatLinks(inserter *jobs.Runner, private bool, lane ai.Completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		store, chats := chatLinks(pool, func() keyvault.Vault { return s.vault }, inserter, 0, private)
		s.chatLinkHandlers = chatlinks.NewHandlers(store, chatsync.NewFiler(pool, store, chats, lane)).WithManifestBase(func() string { return s.send.PublicBaseURL })
		s.channelHandlers = s.WithBotCheck(chatlinks.ProviderDispact, chatsync.BotCheck{HTTP: chatsync.HTTPClient(private), AllowHTTP: private})
	}
}

// WithDispactCalls lets the Dispact app call /mcp; rdb holds each call token id once.
func WithDispactCalls(rdb *redis.Client, private bool) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.dispactCalls = dispactapp.Wire(InstallationDB(pool), rdb, private)
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
		addDeclaredWorker[ChatPullArgs](reg, &chatsync.PullWorker[ChatPullArgs]{Bind: workspaceJobCtx,
			Puller: chatsync.NewPuller(pool, store, chats, cfg.ChatSummaryBrain, log)})
		addDeclaredWorker[ChatPullSweepArgs](reg, &chatPullSweepWorker{pool: pool, log: log})
		return periodicFor(cfg, ChatPullSweepArgs{})
	}
	return nil
}

// chatPullSweepWorker enqueues one chat_pull per live Dispact bot. A failed
// enqueue is logged and the next tick tries again; a failed scan is returned.
type chatPullSweepWorker struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func (w *chatPullSweepWorker) Work(ctx context.Context, _ *river.Job[ChatPullSweepArgs]) error {
	due, scanErr := capture.DueChannelConnections(ctx, w.pool, chatlinks.ProviderDispact)
	for _, d := range due {
		if err := dispatchOne(ctx, w.pool, ChatPullArgs{Workspace: d.WorkspaceID, ConnectionID: d.ID}, nil); err != nil {
			w.log.WarnContext(ctx, "chat pull enqueue failed", "connection", d.ID.String(), "err", err)
		}
	}
	return jobs.FaultContext(ctx, scanErr)
}
