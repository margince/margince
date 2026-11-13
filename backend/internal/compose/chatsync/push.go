// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// The push receiver: Dispact signs a notice when a linked channel has new
// talk, and the notice queues the same chat_pull the sweep queues. A notice
// carries no text, so the pull still reads the history after the link's
// cursor. Each valid notice is marked, and the sweep reads a pushed
// connection hourly instead of every minute (SweepDue).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/webhooks"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The notice rules agreed with Dispact.
const (
	// noticeSkew is how far a notice's timestamp may be off this clock.
	noticeSkew = 5 * time.Minute
	// noticeIDsHeld is how long a webhook-id is refused a second time.
	// Dispact retries a notice for 24 hours under the same id.
	noticeIDsHeld = 24 * time.Hour
)

// The pull cadence. While a connection has had a valid notice within
// PushFresh, the sweep reads it every PushedPullEvery; otherwise every tick.
const (
	PushFresh       = time.Hour
	PushedPullEvery = time.Hour
)

// EventBots answers the live Dispact bot and the secret its notices are
// signed with.
type EventBots interface {
	LiveBotEvents(ctx context.Context, provider string) (capture.LiveBot, string, error)
}

// EventLinks is the slice of the link store a notice acts through.
type EventLinks interface {
	PullLinks(ctx context.Context, connectionID ids.UUID) ([]chatlinks.Link, error)
	MarkChannelGone(ctx context.Context, linkID ids.UUID) error
}

// Replays holds each id once for its time to live; FirstUse is false for an
// id already held.
type Replays interface {
	FirstUse(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

// Marks keeps the last moment under a key; Last answers the zero time when
// none is held.
type Marks interface {
	Mark(ctx context.Context, key string, at time.Time, ttl time.Duration) error
	Last(ctx context.Context, key string) (time.Time, error)
}

// PullEnqueuer queues the chat_pull of one connection. It is unique by its
// args, so ten notices while one pull waits make no second pull.
type PullEnqueuer func(ctx context.Context, workspace, connection ids.UUID) error

// Receiver verifies Dispact's notices and acts on them.
type Receiver struct {
	Bots    EventBots
	Links   EventLinks
	Replays Replays
	Marks   Marks
	Pull    PullEnqueuer
	Now     func() time.Time
	Log     *slog.Logger
}

// noticeKey is where a connection's last valid notice is marked.
func noticeKey(connection ids.UUID) string { return "chat-notice|" + connection.String() }

// sweepKey is where the sweep marks its last pull of a pushed connection.
func sweepKey(connection ids.UUID) string { return "chat-sweep|" + connection.String() }

// Receive implements chatlinks.EventReceiver. ctx carries the installation's
// workspace, which the session middleware binds on every request.
func (r *Receiver) Receive(ctx context.Context, d chatlinks.Delivery) (crmcontracts.DispactChannelEventReceipt, error) {
	ctx = principal.WithActor(ctx, chatLinksPrincipal)
	bot, secret, err := r.Bots.LiveBotEvents(ctx, chatlinks.ProviderDispact)
	if errors.Is(err, capture.ErrNoConnection) || errors.Is(err, capture.ErrNoEventsSecret) {
		return crmcontracts.DispactChannelEventReceipt{}, chatlinks.ErrEventsNotConfigured
	}
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, fmt.Errorf("chat links: reading the events secret: %w", err)
	}
	if err := r.verify(secret, d); err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	var notice crmcontracts.DispactChannelEvent
	if err := json.Unmarshal(d.Body, &notice); err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, fmt.Errorf("%w: %w", chatlinks.ErrEventMalformed, err)
	}
	isCheck := notice.Type == crmcontracts.DispactChannelEventTypeUrlVerification
	if !isCheck && deref(notice.WorkspaceId) != bot.TeamID {
		return crmcontracts.DispactChannelEventReceipt{}, fmt.Errorf("%w: the notice is for another team", chatlinks.ErrEventRejected)
	}
	if err := r.spend(ctx, bot.ConnectionID, d.ID); err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	if isCheck {
		if deref(notice.Challenge) == "" {
			return crmcontracts.DispactChannelEventReceipt{}, fmt.Errorf("%w: a URL check with no challenge", chatlinks.ErrEventMalformed)
		}
		return crmcontracts.DispactChannelEventReceipt{Challenge: notice.Challenge}, nil
	}
	return r.act(ctx, bot.ConnectionID, notice)
}

// verify holds the Standard Webhooks rules: a signature made with the secret,
// and a timestamp within noticeSkew of this clock.
func (r *Receiver) verify(secret string, d chatlinks.Delivery) error {
	ts, err := strconv.ParseInt(d.Timestamp, 10, 64)
	if err != nil || d.ID == "" {
		return fmt.Errorf("%w: webhook-id and a unix webhook-timestamp are required", chatlinks.ErrEventRejected)
	}
	if off := r.Now().Sub(time.Unix(ts, 0)).Abs(); off > noticeSkew {
		return fmt.Errorf("%w: the timestamp is %s off", chatlinks.ErrEventRejected, off)
	}
	ok, err := webhooks.Verify(secret, d.ID, ts, d.Body, d.Signature)
	if err != nil {
		return fmt.Errorf("chat links: the sealed events secret cannot sign: %w", err)
	}
	if !ok {
		return fmt.Errorf("%w: no signature matches", chatlinks.ErrEventRejected)
	}
	return nil
}

// spend holds the webhook-id for noticeIDsHeld, so a replayed notice is
// refused. A store that cannot answer refuses the notice, and Dispact retries.
func (r *Receiver) spend(ctx context.Context, connection ids.UUID, id string) error {
	first, err := r.Replays.FirstUse(ctx, "dispact-notice|"+connection.String()+"|"+id, noticeIDsHeld)
	if err != nil {
		return fmt.Errorf("%w: %w", chatlinks.ErrEventsUnavailable, err)
	}
	if !first {
		return chatlinks.ErrEventReplayed
	}
	return nil
}

// act marks the notice, then queues the pull or pauses the archived
// channel's link. The mark comes first, so a pull that is running already
// sees it when it ends and runs once more (PullWorker).
func (r *Receiver) act(ctx context.Context, connection ids.UUID, notice crmcontracts.DispactChannelEvent) (crmcontracts.DispactChannelEventReceipt, error) {
	if err := r.Marks.Mark(ctx, noticeKey(connection), r.Now(), PushFresh); err != nil {
		r.Log.WarnContext(ctx, "chat links: the notice was not marked; the sweep keeps reading every minute", "err", err)
	}
	link, found, err := r.linkOf(ctx, connection, deref(notice.ChannelSlug))
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	if !found {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
	switch notice.Type {
	case crmcontracts.DispactChannelEventTypeChannelArchived:
		if err := r.Links.MarkChannelGone(ctx, link.ID); err != nil {
			return crmcontracts.DispactChannelEventReceipt{}, fmt.Errorf("chat links: pausing the archived channel's link: %w", err)
		}
		return receipt(crmcontracts.DispactChannelEventReceiptStatusLinkPaused), nil
	case crmcontracts.DispactChannelEventTypeMessageCreated, crmcontracts.DispactChannelEventTypeThreadReplied,
		crmcontracts.DispactChannelEventTypeMessageUpdated:
		ws, ok := principal.WorkspaceID(ctx)
		if !ok {
			return crmcontracts.DispactChannelEventReceipt{}, errors.New("chat links: a notice outside a workspace context")
		}
		if err := r.Pull(ctx, ws, connection); err != nil {
			return crmcontracts.DispactChannelEventReceipt{}, fmt.Errorf("%w: queueing the pull: %w", chatlinks.ErrEventsUnavailable, err)
		}
		return receipt(crmcontracts.DispactChannelEventReceiptStatusPullQueued), nil
	default:
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
}

// linkOf finds the active link of slug among the connection's own links, so
// a notice can only ever reach this bot's channels.
func (r *Receiver) linkOf(ctx context.Context, connection ids.UUID, slug string) (chatlinks.Link, bool, error) {
	links, err := r.Links.PullLinks(ctx, connection)
	if err != nil {
		return chatlinks.Link{}, false, fmt.Errorf("chat links: reading the bot's links: %w", err)
	}
	for _, link := range links {
		if slug != "" && link.ChannelSlug == slug {
			return link, true, nil
		}
	}
	return chatlinks.Link{}, false, nil
}

func receipt(status crmcontracts.DispactChannelEventReceiptStatus) crmcontracts.DispactChannelEventReceipt {
	return crmcontracts.DispactChannelEventReceipt{Status: &status}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SweepDue decides whether chat_pull_sweep reads a connection now. A
// connection with no valid notice within PushFresh is read on every tick. A
// pushed one is read once PushedPullEvery has passed since the sweep last
// read it. A push that stops therefore falls back to polling within PushFresh.
func SweepDue(now, lastNotice, lastSweep time.Time) bool {
	if now.Sub(lastNotice) >= PushFresh {
		return true
	}
	return now.Sub(lastSweep) >= PushedPullEvery
}

// LiveEventBots reads the live bot's events secret through a vault the api
// learns from a later option, so it is read per notice.
type LiveEventBots struct {
	DB    *database.DB
	Vault func() keyvault.Vault
}

// LiveBotEvents implements EventBots.
func (b LiveEventBots) LiveBotEvents(ctx context.Context, provider string) (capture.LiveBot, string, error) {
	vault := b.Vault()
	if vault == nil {
		return capture.LiveBot{}, "", capture.ErrNoEventsSecret
	}
	return capture.NewRegistry(b.DB, nil, nil, vault).LiveBotEvents(ctx, provider)
}
