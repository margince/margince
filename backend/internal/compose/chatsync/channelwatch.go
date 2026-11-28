// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (p *Puller) maintainChannelWatch(ctx context.Context, session chatlinks.Session, link chatlinks.Link) error {
	bot, ok := teamBot(session.Chat, link.ChannelSlug)
	if !ok || p.NotificationURL == "" {
		return nil
	}
	watch, err := p.prepareNativeWatch(ctx, bot, link)
	if err != nil {
		return err
	}
	if watch.Expires != nil && watch.Expires.After(p.Now().Add(12*time.Hour)) && !watch.Reauthorize && !watch.Rescan && watch.Error == nil {
		return nil
	}
	secret, err := p.Store.ChannelWatchSecret(ctx, watch)
	if err != nil {
		return err
	}
	now := p.Now()
	watch, err = p.Store.BeginChannelWatchAttempt(ctx, watch, now.Add(teams.WatchMaxLifetime))
	if err != nil {
		return err
	}
	subscription, err := bot.EnsureWatch(ctx, teams.WatchRequest{Channel: watch.Channel, URL: watch.URL, Secret: secret, Subscription: watch.Subscription, Now: now, Reauthorize: watch.Reauthorize})
	reason := ""
	if err != nil {
		reason, _ = nativeRootFailure(err)
	}
	recorded := p.Store.CompleteChannelWatch(ctx, watch, subscription.ID, subscription.Expires, reason)
	return errors.Join(err, recorded)
}

// WithNotificationOrigin binds Teams subscriptions to the configured callback origin.
func (p *Puller) WithNotificationOrigin(origin string) *Puller {
	if origin != "" {
		p.NotificationURL = strings.TrimRight(origin, "/") + "/v1/channel-events/teams/graph"
	}
	return p
}

func (p *Puller) retireChannelWatches(ctx context.Context, connection ids.UUID) error {
	watches, err := p.Store.UnusedChannelWatches(ctx, connection)
	if err != nil || len(watches) == 0 {
		return err
	}

	var session chatlinks.Session
	var opening error
	deferred := []error{}
	for _, watch := range watches {
		if watch.LeaseEnded(p.Now()) {
			if err := p.Store.RetireChannelWatch(ctx, watch, p.Now(), false); err != nil {
				deferred = append(deferred, err)
			}
			continue
		}
		if session.Chat == nil && opening == nil {
			session, opening = chatlinks.OpenProvider(ctx, p.Chats, chatlinks.ProviderTeams)
			if opening == nil && session.ConnectionID != connection {
				opening = chatlinks.ErrBotNotConnected
			}
			if opening != nil {
				deferred = append(deferred, opening)
			}
		}
		if opening != nil {
			continue
		}
		bot, ok := teamBot(session.Chat, watch.Channel)
		if !ok {
			deferred = append(deferred, messaging.ErrUnsupported)
			continue
		}
		if err := p.retireNativeWatch(ctx, bot, watch); err != nil {
			deferred = append(deferred, err)
		}
	}
	return errors.Join(deferred...)
}

func (p *Puller) retireNativeWatch(ctx context.Context, bot *teams.Bot, watch chatlinks.ChannelWatch) error {
	app, tenant := bot.WatchBinding()
	if app != watch.App || tenant != watch.Tenant {
		return messaging.ErrUnauthorized
	}
	secret, err := p.Store.ChannelWatchSecret(ctx, watch)
	if err != nil {
		return err
	}
	if err := bot.RemoveWatch(ctx, teams.WatchRequest{Channel: watch.Channel, URL: watch.URL, Secret: secret, Subscription: watch.Subscription, Now: p.Now()}); err != nil {
		return err
	}
	return p.Store.RetireChannelWatch(ctx, watch, p.Now(), true)
}

func (p *Puller) moveChannelWatch(ctx context.Context, bot *teams.Bot, watch chatlinks.ChannelWatch) error {
	secret, err := p.Store.ChannelWatchSecret(ctx, watch)
	if err != nil {
		return err
	}
	if err := bot.RemoveWatch(ctx, teams.WatchRequest{Channel: watch.Channel, URL: watch.URL, Secret: secret, Subscription: watch.Subscription, Now: p.Now()}); err != nil {
		return err
	}
	return p.Store.ReconfigureChannelWatch(ctx, watch, p.NotificationURL)
}

func (p *Puller) prepareNativeWatch(ctx context.Context, bot *teams.Bot, link chatlinks.Link) (chatlinks.ChannelWatch, error) {
	app, tenant := bot.WatchBinding()
	watch, err := p.Store.PrepareChannelWatch(ctx, link, app, tenant, p.NotificationURL, p.Now())
	if err != nil && !watch.ID.IsZero() && watch.App == app && watch.Tenant == tenant && watch.URL != p.NotificationURL {
		if moveErr := p.moveChannelWatch(ctx, bot, watch); moveErr != nil {
			return watch, moveErr
		}
		watch, err = p.Store.PrepareChannelWatch(ctx, link, app, tenant, p.NotificationURL, p.Now())
	}
	if err != nil {
		if !watch.ID.IsZero() {
			return watch, errors.Join(err, p.Store.CompleteChannelWatch(ctx, watch, "", time.Time{}, "source_changed"))
		}
		return watch, err
	}
	return watch, nil
}
