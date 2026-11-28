// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/compose/dispactapp"
	"github.com/margince/margince/backend/internal/compose/messengeraccounts"
	"github.com/margince/margince/backend/internal/compose/oidctoken"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// ChatLinkFlushArgs posts what one chat link has pending and refreshes its card.
type ChatLinkFlushArgs struct {
	LinkID ids.UUID `json:"link_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ChatLinkFlushArgs) Kind() string { return "chat_link_flush" }

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
	ConnectionID ids.UUID `json:"connection_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ChatPullArgs) Kind() string { return "chat_pull" }

// Connection names the bot whose channels the pull reads.
func (a ChatPullArgs) Connection() ids.UUID { return a.ConnectionID }

// InsertOpts makes one pull per bot wait or run at a time, whoever enqueues
// it, because two pulls would judge the same talk.
func (ChatPullArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: sweptJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	}
}

func chatLinks(pool *pgxpool.Pool, vault func() keyvault.Vault, in chatsync.TxInserter, delay time.Duration, private bool) (*chatlinks.Store, *chatsync.LiveChats) {
	return chatsync.Wire(InstallationDB(pool), vault, chatsync.Enqueuer{
		Inserter: in, Delay: delay, MaxAttempts: oneOffJobMaxAttempts, States: activeSweepStates,
		Job:  func(link ids.UUID) river.JobArgs { return ChatLinkFlushArgs{LinkID: link} },
		Pull: func(bot ids.UUID) river.JobArgs { return ChatPullArgs{ConnectionID: bot} },
	}, private)
}

// WithChatLinks composes /channel-links and the Dispact bot connect; it must
// follow WithChannelSurface. A new link's card is scheduled with no window.
// lane is the chat_summary lane a chosen message is summarised on.
func WithChatLinks(inserter *jobs.Runner, private bool, lane ai.Completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		store, chats := chatLinks(pool, func() keyvault.Vault { return s.vault }, inserter, 0, private)
		chats.App = s.messagingAppCredentials
		registry := capture.NewRegistry(InstallationDB(pool), nil, nil, nil)
		binding := func(ctx context.Context) (capture.LiveBot, error) {
			return registry.LiveBotIdentity(ctx, chatlinks.ProviderDispact)
		}
		s.dispact = binding
		s.installed = registry.LiveBotIdentity
		s.historyProfile = chats.HistoryProfile
		store.WithHistoryProfiles(chats.HistoryProfile)
		s.http = chatsync.HTTPClient(private)
		store.WithMessenger(func() keyvault.Vault { return s.vault }, messengeraccounts.Clients(private, registry.LiveBotIdentity))
		s.messengerOAuthHandlers.store = store
		store.WithMessengerRoster(messengeraccounts.RecordRoster)
		store.WithMemberAuthority(messengeraccounts.MemberAuthority)
		store.WithMemberRefresh(func(ctx context.Context, grant chatlinks.MemberGrant, raw []byte) ([]byte, error) {
			h := s.messengerOAuthHandlers
			if h.resolve == nil {
				return raw, nil
			}
			return messengeraccounts.RefreshCredential(ctx, grant, raw, h.resolve, h.now)
		})
		s.chatSourceHandlers = chatSourceHandlers{pool: pool, notes: activities.NewStore(InstallationDB(pool)), links: store, now: time.Now}
		s.chatPulls = inserter
		filer := chatsync.NewFiler(pool, store, chats, lane)
		filer.Reconfirm = channelReconfirmation(pool)
		filer.Reauthorize = s.channelFilingAuthority(pool)
		filer.PreviewOrigin = func() string { return s.send.PublicBaseURL }
		s.chatLinkHandlers = chatlinks.NewHandlers(store, filer).WithDeliveryPreviewer(filer).WithManifestBase(func() string { return s.send.PublicBaseURL })
		s.channelHandlers = s.WithBotCheck(chatlinks.ProviderDispact, chatsync.BotCheck{HTTP: chatsync.HTTPClient(private), AllowHTTP: private})
		s.channelHandlers = s.WithBotCheck(chatlinks.ProviderSlack, chatsync.SlackBotCheck{HTTP: chatsync.HTTPClient(private)})
		s.channelHandlers = s.WithBotCheck(chatlinks.ProviderTeams, chatsync.TeamsBotCheck{HTTP: chatsync.HTTPClient(private), App: s.messagingAppCredentials})
	}
}

func (s *Server) channelFilingAuthority(pool *pgxpool.Pool) func(context.Context) (context.Context, error) {
	resolver := identity.NewService(pool)
	return func(ctx context.Context) (context.Context, error) {
		actor, ok := principal.Actor(ctx)
		if !ok {
			return nil, apperrors.ErrPermissionDenied
		}
		if actor.Type == principal.PrincipalHuman {
			rbac, seat, err := resolver.AdmittedAuthority(ctx, actor.UserID, ids.Nil)
			if err != nil {
				return nil, err
			}
			return principal.WithActor(ctx, authz.WithAuthority(actor, rbac, seat)), nil
		}
		if actor.Type != principal.PrincipalAgent || s.toolRegistry == nil {
			return nil, apperrors.ErrPermissionDenied
		}
		for _, spec := range s.toolRegistry.Specs() {
			if spec.Name == fileChannelMessageTool {
				return auth.NewGate(resolver, auth.WithVolumeMeter(s.volumeMeter)).Admit(ctx, spec, nil)
			}
		}
		return nil, apperrors.ErrPermissionDenied
	}
}

// WithDispactCalls lets the Dispact app call /mcp; replays holds each token id once.
func WithDispactCalls(replays dispactapp.Replays, private bool) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.dispactCalls = dispactapp.Wire(InstallationDB(pool), replays, private).SetLog(s.log)
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
		apps := capture.NewConnectorAppStore(NewSettingsStore(pool), cfg.ChannelVault, log)
		chats.App = func(ctx context.Context, provider string) (capture.ConnectorApp, error) {
			return resolveMessagingApp(ctx, provider, cfg.MessagingGraph, apps)
		}
		flusher := chatsync.NewFlusher(pool, chats, store, cfg.SendOrigin.PublicBaseURL, log)
		flusher.Lane = cfg.ChatSummaryBrain
		addDeclaredWorker[ChatLinkFlushArgs](reg, &chatsync.Worker[ChatLinkFlushArgs]{
			Flusher: flusher,
		})
		puller := chatsync.NewPuller(pool, store, chats, cfg.ChatSummaryBrain, log).WithNotificationOrigin(cfg.SendOrigin.PublicBaseURL)
		puller.Reconfirm = channelReconfirmation(pool)
		addDeclaredWorker[ChatPullArgs](reg, &chatsync.PullWorker[ChatPullArgs]{
			Puller: puller, Marks: cfg.ChatPushMarks,
		})
		addDeclaredWorker[ChatPullSweepArgs](reg, &chatPullSweepWorker{
			pool: pool, log: log,
			cadence: chatsync.Sweeper{Marks: cfg.ChatPushMarks, Now: time.Now, Log: log},
		})
		return periodicFor(cfg, ChatPullSweepArgs{})
	}
	return nil
}

// chatPullSweepWorker enqueues one chat_pull per live bot the cadence
// finds due. A failed enqueue is logged and the next tick tries again; a
// failed scan is returned.
type chatPullSweepWorker struct {
	pool    *pgxpool.Pool
	log     *slog.Logger
	cadence chatsync.Sweeper
}

func (w *chatPullSweepWorker) Work(ctx context.Context, _ *river.Job[ChatPullSweepArgs]) error {
	var scanErr error
	cleanup, err := chatlinks.DueChannelWatchCleanup(ctx, w.pool)
	scanErr = errors.Join(scanErr, err)
	for _, work := range cleanup {
		scanErr = errors.Join(scanErr, dispatchOne(ctx, w.pool, ChatPullArgs{ConnectionID: work.Connection}, nil))
	}
	for _, provider := range []string{chatlinks.ProviderDispact, chatlinks.ProviderSlack, chatlinks.ProviderTeams} {
		due, err := capture.DueChannelConnections(ctx, w.pool, provider)
		scanErr = errors.Join(scanErr, err)
		for _, d := range due {
			if !w.cadence.Due(ctx, d.ID) {
				continue
			}
			if err := dispatchOne(ctx, w.pool, ChatPullArgs{ConnectionID: d.ID}, nil); err != nil {
				w.log.WarnContext(ctx, "chat pull enqueue failed", "connection", d.ID.String(), "err", err)
				continue
			}
			w.cadence.Swept(ctx, d.ID)
		}
	}
	return jobs.FaultContext(ctx, scanErr)
}

// WithChatEvents composes Dispact's push receiver. A notice queues the
// chat_pull through the inserter WithChatLinks holds. It is read per notice, so
// the two options may come in any order. replays holds each notice id once.
// marks keeps when the last valid notice came, for the sweep's cadence.
func WithChatEvents(replays chatsync.Replays, marks chatsync.Marks) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		store, _ := chatLinks(pool, func() keyvault.Vault { return s.vault }, nil, 0, false)
		receiver := &chatsync.Receiver{
			Bots:  chatsync.LiveEventBots{DB: InstallationDB(pool), Vault: func() keyvault.Vault { return s.vault }},
			Links: store, Roots: store, Replays: replays, Marks: marks, Now: time.Now, Log: s.log,
			Pull: func(ctx context.Context, connection ids.UUID) error {
				if s.chatPulls == nil {
					return errors.New("chat links: this role queues no chat_pull")
				}
				args := ChatPullArgs{ConnectionID: connection}
				return s.chatPulls.Enqueue(ctx, args, jobs.QueuedAsArgs(args, nil))
			},
		}
		registry := capture.NewRegistry(InstallationDB(pool), nil, nil, nil)
		teamsReceiver := &chatsync.TeamsReceiver{Receiver: receiver, Store: store, App: s.messagingAppCredentials, HTTP: chatsync.HTTPClient(false), Verifier: oidctoken.NewBotFramework(), Binding: func(ctx context.Context) (capture.LiveBot, error) {
			return registry.LiveBotIdentity(ctx, chatlinks.ProviderTeams)
		}}
		s.chatEventHandlers = chatlinks.NewEventHandlers(receiver).WithSlack(&chatsync.SlackReceiver{Receiver: receiver, Roots: store}).WithTeams(teamsReceiver).WithTeamsGraph(&chatsync.TeamsGraphReceiver{Receiver: receiver, Store: store, App: s.messagingAppCredentials})
	}
}
