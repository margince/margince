// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
)

type teamChats struct {
	*teams.Bot
	bots map[string]*teams.Bot
}

func teamBot(chat chatlinks.Chat, channel string) (*teams.Bot, bool) {
	if bot, ok := chat.(*teams.Bot); ok {
		return bot, true
	}
	if chat, ok := chat.(*teamChats); ok {
		team, _, valid := strings.Cut(channel, "/")
		bot, found := chat.bots[team]
		return bot, valid && found
	}
	return nil, false
}

func (t *teamChats) Me(ctx context.Context) (chatwire.Bot, error) {
	var unavailable error
	for _, bot := range t.bots {
		identity, err := bot.Me(ctx)
		if err == nil {
			return identity, nil
		}
		if !errors.Is(err, chatwire.ErrNotMember) && !errors.Is(err, messaging.ErrNotFound) {
			unavailable = err
		}
	}
	if unavailable != nil {
		return chatwire.Bot{}, unavailable
	}
	return chatwire.Bot{}, chatwire.ErrNotMember
}

func (t *teamChats) Channels(ctx context.Context) ([]chatwire.Channel, error) {
	out := []chatwire.Channel{}
	for _, bot := range t.bots {
		identity, err := bot.Me(ctx)
		if errors.Is(err, chatwire.ErrNotMember) || errors.Is(err, messaging.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		channels, err := bot.Channels(ctx)
		if errors.Is(err, chatwire.ErrNotMember) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, channel := range channels {
			channel.Name = identity.Name + " / " + channel.Name
			out = append(out, channel)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

func (t *teamChats) Channel(ctx context.Context, id string) (chatwire.Channel, error) {
	bot, ok := teamBot(t, id)
	if !ok {
		return chatwire.Channel{}, chatwire.ErrNotMember
	}
	return bot.Channel(ctx, id)
}

func (t *teamChats) Audience(ctx context.Context, id string) (messaging.Conversation, error) {
	bot, ok := teamBot(t, id)
	if !ok {
		return messaging.Conversation{}, chatwire.ErrNotMember
	}
	return bot.Audience(ctx, id)
}

func (t *teamChats) Roots(ctx context.Context, id string, since time.Time, cursor string) (chatwire.RootPage, error) {
	bot, ok := teamBot(t, id)
	if !ok {
		return chatwire.RootPage{}, chatwire.ErrNotMember
	}
	return bot.Roots(ctx, id, since, cursor)
}

func (t *teamChats) Thread(ctx context.Context, id, root, cursor string) (chatwire.ThreadPage, error) {
	bot, ok := teamBot(t, id)
	if !ok {
		return chatwire.ThreadPage{}, chatwire.ErrNotMember
	}
	return bot.Thread(ctx, id, root, cursor)
}

func (t *teamChats) Post(ctx context.Context, id, text, key, root string) (chatwire.Posted, error) {
	bot, ok := teamBot(t, id)
	if !ok {
		return chatwire.Posted{}, chatwire.ErrNotMember
	}
	return bot.Post(ctx, id, text, key, root)
}

func (t *teamChats) PostReviewed(ctx context.Context, id, text, key, root, expected string) (chatwire.Posted, error) {
	bot, ok := teamBot(t, id)
	if !ok {
		return chatwire.Posted{}, chatwire.ErrNotMember
	}
	return bot.PostReviewed(ctx, id, text, key, root, expected)
}

func (t *teamChats) Update(ctx context.Context, id, message, text string) error {
	bot, ok := teamBot(t, id)
	if !ok {
		return chatwire.ErrNotMember
	}
	return bot.Update(ctx, id, message, text)
}

func (t *teamChats) UpdateReviewed(ctx context.Context, id, message, text, expected string) error {
	bot, ok := teamBot(t, id)
	if !ok {
		return chatwire.ErrNotMember
	}
	return bot.UpdateReviewed(ctx, id, message, text, expected)
}

func (t *teamChats) LocateMessage(ctx context.Context, channel, reference string) (chatwire.MessageLocation, error) {
	bot, ok := teamBot(t, channel)
	if !ok {
		return chatwire.MessageLocation{}, chatwire.ErrNotMember
	}
	return bot.LocateMessage(ctx, channel, reference)
}

var (
	_ chatwire.RootReader     = (*teamChats)(nil)
	_ chatwire.MessageLocator = (*teamChats)(nil)
)
