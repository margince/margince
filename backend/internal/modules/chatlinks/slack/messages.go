// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// Messages reads provider content through the current participant credential.
func (c *Client) Messages(ctx context.Context, conversation, root, cursor string) (messaging.Page, error) {
	if _, err := c.Conversation(ctx, conversation); err != nil {
		return messaging.Page{}, err
	}
	method := "conversations.history"
	fields := url.Values{channelKey: {conversation}, limitKey: {"15"}, "cursor": {cursor}}
	if root != "" {
		method = repliesMethod
		fields.Set("ts", root)
	}
	raw, err := c.call(ctx, http.MethodGet, method+"?"+fields.Encode(), nil)
	if err != nil {
		return messaging.Page{}, err
	}
	var page struct {
		More     bool `json:"has_more"`
		Messages []struct {
			ID     string `json:"ts"`
			Root   string `json:"thread_ts"`
			Author string `json:"user"`
			Bot    string `json:"bot_id"`
			Text   string `json:"text"`
			Edited *struct {
				TS string `json:"ts"`
			} `json:"edited"`
		} `json:"messages"`
		Metadata struct {
			Next string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return messaging.Page{}, messaging.ErrUnavailable
	}
	if page.More && page.Metadata.Next == "" {
		return messaging.Page{}, messaging.ErrUnavailable
	}
	out := messaging.Page{Messages: []messaging.Message{}, Next: page.Metadata.Next}
	if root != "" {
		out.NextDirection = "newer"
	}
	for _, row := range page.Messages {
		at, err := timestamp(row.ID)
		if err != nil {
			return messaging.Page{}, messaging.ErrUnavailable
		}
		author := row.Author
		if author == "" {
			author = row.Bot
		}
		out.Messages = append(out.Messages, messaging.Message{ID: row.ID, Root: row.Root, Author: author, Text: row.Text, At: at, Edited: row.Edited != nil})
	}
	return out, nil
}

// Send uses the participant's identity for a reviewed personal message.
func (c *Client) Send(ctx context.Context, in messaging.Send) (messaging.Receipt, error) {
	if strings.TrimSpace(in.Text) == "" || utf8.RuneCountInString(in.Text) > 40000 || in.RequestID == "" {
		return messaging.Receipt{}, messaging.ErrInvalid
	}
	audience, err := c.Conversation(ctx, in.Conversation)
	if err != nil {
		return messaging.Receipt{}, err
	}
	key, keyErr := messaging.AudienceKey(audience)
	if keyErr != nil || in.ExpectedAudience == "" || key != in.ExpectedAudience {
		return messaging.Receipt{}, messaging.ErrInvalid
	}
	if !audience.AudienceKnown || audience.External {
		return messaging.Receipt{}, messaging.ErrInvalid
	}
	body, err := json.Marshal(struct {
		Channel     string `json:"channel"`
		Text        string `json:"text"`
		Root        string `json:"thread_ts,omitempty"`
		ClientID    string `json:"client_msg_id"`
		Markdown    bool   `json:"mrkdwn"`
		Parse       string `json:"parse"`
		UnfurlLinks bool   `json:"unfurl_links"`
		UnfurlMedia bool   `json:"unfurl_media"`
	}{Channel: in.Conversation, Text: in.Text, Root: in.Root, ClientID: in.RequestID, Parse: "none"})
	if err != nil {
		return messaging.Receipt{}, messaging.ErrInvalid
	}
	raw, err := c.call(ctx, http.MethodPost, "chat.postMessage", body)
	if err != nil {
		return messaging.Receipt{}, err
	}
	var response struct {
		Channel string `json:"channel"`
		ID      string `json:"ts"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Channel != in.Conversation || response.ID == "" {
		return messaging.Receipt{}, messaging.ErrDeliveryUnknown
	}
	return messaging.Receipt{Conversation: response.Channel, ID: response.ID}, nil
}

func timestamp(value string) (time.Time, error) {
	seconds, fraction, ok := strings.Cut(value, ".")
	if !ok || len(fraction) > 9 {
		return time.Time{}, messaging.ErrInvalid
	}
	sec, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return time.Time{}, messaging.ErrInvalid
	}
	nano, err := strconv.ParseInt(fraction+strings.Repeat("0", 9-len(fraction)), 10, 64)
	if err != nil {
		return time.Time{}, messaging.ErrInvalid
	}
	return time.Unix(sec, nano).UTC(), nil
}
