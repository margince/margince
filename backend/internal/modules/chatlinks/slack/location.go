// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// LocateMessage resolves a message reference within the selected native channel.
func (b *Bot) LocateMessage(ctx context.Context, channel, reference string) (chatwire.MessageLocation, error) {
	selected, root, err := slackMessageReference(channel, reference)
	if err != nil {
		return chatwire.MessageLocation{}, err
	}
	if _, err := timestamp(selected); err != nil {
		return chatwire.MessageLocation{}, messaging.ErrInvalid
	}
	if root == "" {
		page, err := b.readMessages(ctx, "conversations.replies", url.Values{channelKey: {channel}, "ts": {selected}, limitKey: {"1"}})
		if err != nil {
			return chatwire.MessageLocation{}, err
		}
		if len(page.Messages) == 0 {
			return chatwire.MessageLocation{}, chatwire.ErrUnreachable
		}
		root = page.Messages[0].Root
		if root == "" {
			root = page.Messages[0].ID
		}
	}
	if _, err := timestamp(root); err != nil {
		return chatwire.MessageLocation{}, messaging.ErrInvalid
	}
	return chatwire.MessageLocation{Message: selected, Root: root}, nil
}

func slackMessageReference(channel, reference string) (string, string, error) {
	selected := strings.TrimSpace(reference)
	root := ""
	if strings.Contains(selected, "://") {
		link, err := url.Parse(selected)
		if err != nil || link.Scheme != "https" || link.User != nil || !strings.HasSuffix(link.Host, ".slack.com") {
			return "", "", messaging.ErrInvalid
		}
		prefix := "/archives/" + channel + "/p"
		stamp, ok := strings.CutPrefix(link.Path, prefix)
		if !ok || len(stamp) < 7 || len(stamp) > 24 || strings.Trim(stamp, "0123456789") != "" {
			return "", "", messaging.ErrInvalid
		}
		selected = stamp[:len(stamp)-6] + "." + stamp[len(stamp)-6:]
		root = link.Query().Get("thread_ts")
		if cid := link.Query().Get("cid"); cid != "" && cid != channel {
			return "", "", messaging.ErrInvalid
		}
	}
	return selected, root, nil
}
