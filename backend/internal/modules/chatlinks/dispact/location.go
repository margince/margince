// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LocateMessage resolves a message reference within the selected native channel.
func (c *CanonicalClient) LocateMessage(ctx context.Context, channel, reference string) (chatwire.MessageLocation, error) {
	room, err := c.Channel(ctx, channel)
	if err != nil {
		return chatwire.MessageLocation{}, err
	}
	ref, err := c.messageReference(room, reference)
	if err != nil {
		return chatwire.MessageLocation{}, err
	}
	if message, err := ids.Parse(ref); err == nil {
		return c.locateImmutableMessage(ctx, room.NativeID, message.String())
	}
	seq, ok := MessageSequence(ref, room.Name, false)
	if !ok {
		seq, err = seqOf(ref)
	}
	if err != nil || seq <= 0 {
		return chatwire.MessageLocation{}, messaging.ErrInvalid
	}
	page, err := c.History(ctx, room.NativeID, seq-1, 1)
	if err != nil {
		return chatwire.MessageLocation{}, err
	}
	for _, message := range page.Messages {
		if message.Seq == seq && !message.Deleted && message.ChannelID == room.NativeID && message.NativeID != "" && message.RootID != "" {
			return chatwire.MessageLocation{Message: message.NativeID, Root: message.RootID}, nil
		}
	}
	return chatwire.MessageLocation{}, messaging.ErrInvalid
}

func (c *CanonicalClient) messageReference(room Channel, reference string) (string, error) {
	ref := strings.TrimSpace(reference)
	if strings.Contains(ref, "://") {
		link, err := url.Parse(ref)
		origin, originErr := url.Parse(c.address)
		if err != nil || originErr != nil || link.Scheme != origin.Scheme || link.Host != origin.Host || link.User != nil {
			return "", messaging.ErrInvalid
		}
		parts := strings.Split(strings.Trim(link.Path, "/"), "/")
		if len(parts) != 4 || parts[0] != "channels" || parts[2] != "messages" || parts[1] != immutableChannelRef(room.NativeID) && parts[1] != room.Name {
			return "", messaging.ErrInvalid
		}
		return parts[3], nil
	}

	return ref, nil
}

func (c *CanonicalClient) locateImmutableMessage(ctx context.Context, channel, message string) (chatwire.MessageLocation, error) {
	var source struct {
		Channel string `json:"channel_id"`
		Message string `json:"message_id"`
		Root    string `json:"root_message_id"`
	}
	path := "/api/bot/channels/" + url.PathEscape(immutableChannelRef(channel)) + "/messages/" + url.PathEscape(message) + "/source"
	if err := c.call(ctx, http.MethodGet, path, nil, &source); err != nil {
		var refusal *CodeError
		if errors.As(err, &refusal) && refusal.Code == "message_not_found" {
			return chatwire.MessageLocation{}, messaging.ErrInvalid
		}
		return chatwire.MessageLocation{}, err
	}
	if source.Channel != channel || source.Message != message {
		return chatwire.MessageLocation{}, chatwire.ErrUnreachable
	}
	if _, err := ids.Parse(source.Root); err != nil {
		return chatwire.MessageLocation{}, chatwire.ErrUnreachable
	}
	return chatwire.MessageLocation{Message: source.Message, Root: source.Root}, nil
}
