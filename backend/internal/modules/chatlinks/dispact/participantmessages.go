// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

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

type participantMessage struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace_id"`
	Channel   string    `json:"channel_id"`
	Author    string    `json:"author_id"`
	Content   string    `json:"content"`
	Seq       int64     `json:"seq"`
	Thread    string    `json:"thread_id"`
	At        time.Time `json:"created_at"`
	Edited    bool      `json:"edited"`
	Deleted   bool      `json:"is_deleted"`
}

func (c *Participant) message(ctx context.Context, room participantRoom, id string) (participantMessage, error) {
	if !participantID.MatchString(id) {
		return participantMessage{}, messaging.ErrInvalid
	}
	raw, err := c.http.Call(ctx, http.MethodGet, "/messages/"+url.PathEscape(id), nil)
	if err != nil {
		return participantMessage{}, err
	}
	var message participantMessage
	if json.Unmarshal(raw, &message) != nil || message.ID != id || message.Workspace != c.workspace || message.Channel != room.ID {
		return participantMessage{}, messaging.ErrUnavailable
	}
	return message, nil
}

// Messages reads provider content through the current participant credential.
func (c *Participant) Messages(ctx context.Context, slug, root, cursor string) (messaging.Page, error) {
	room, err := c.channel(ctx, slug)
	if err != nil {
		return messaging.Page{}, err
	}
	values := url.Values{limitKey: {"50"}, "with_has_more": {"true"}}
	if cursor != "" {
		seq, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || seq < 1 {
			return messaging.Page{}, messaging.ErrInvalid
		}
		values.Set("before_seq", cursor)
	}
	path := "/channels/" + url.PathEscape(immutableChannelRef(room.ID)) + "/messages"
	var parent participantMessage
	if root != "" {
		parent, err = c.message(ctx, room, root)
		if err != nil {
			return messaging.Page{}, err
		}
		if parent.Deleted {
			return messaging.Page{}, messaging.ErrUnavailable
		}
		if parent.Thread == "" {
			return participantPage([]participantMessage{parent}, false, root, room, c.workspace)
		}
		path = "/threads/" + url.PathEscape(parent.Thread) + "/replies"
	}
	raw, err := c.http.Call(ctx, http.MethodGet, path+"?"+values.Encode(), nil)
	if err != nil {
		return messaging.Page{}, err
	}
	var page struct {
		Messages []participantMessage `json:"messages"`
		Replies  []participantMessage `json:"replies"`
		Parent   *participantMessage  `json:"parent_message"`
		More     bool                 `json:"has_more"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return messaging.Page{}, messaging.ErrUnavailable
	}
	rows := page.Messages
	if root != "" {
		if page.Parent == nil || page.Parent.ID != root || page.Parent.Channel != room.ID {
			return messaging.Page{}, messaging.ErrUnavailable
		}
		rows = append([]participantMessage{*page.Parent}, page.Replies...)
	}
	return participantPage(rows, page.More, root, room, c.workspace)
}

func participantPage(rows []participantMessage, more bool, root string, room participantRoom, workspace string) (messaging.Page, error) {
	out := messaging.Page{Messages: []messaging.Message{}}
	oldest := int64(0)
	for _, row := range rows {
		if row.ID == "" || row.Channel != room.ID || row.Workspace != workspace || row.Seq < 1 || row.At.IsZero() {
			return messaging.Page{}, messaging.ErrUnavailable
		}
		// A thread's parent is context, never its older-replies cursor.
		if row.ID != root && (oldest == 0 || row.Seq < oldest) {
			oldest = row.Seq
		}
		text := row.Content
		if row.Deleted {
			text = ""
		}
		out.Messages = append(out.Messages, messaging.Message{ID: row.ID, Root: root, Author: row.Author, Text: text, At: row.At, Edited: row.Edited, Deleted: row.Deleted})
	}
	if more {
		if oldest == 0 {
			return messaging.Page{}, messaging.ErrUnavailable
		}
		out.Next = strconv.FormatInt(oldest, 10)
	}
	return out, nil
}

// Send uses the participant's identity for a reviewed personal message.
func (c *Participant) Send(ctx context.Context, in messaging.Send) (messaging.Receipt, error) {
	if !validParticipantSend(in) {
		return messaging.Receipt{}, messaging.ErrInvalid
	}
	audience, err := c.Conversation(ctx, in.Conversation)
	if err != nil {
		return messaging.Receipt{}, err
	}
	key, err := messaging.AudienceKey(audience)
	if err != nil || audience.External || key != in.ExpectedAudience {
		return messaging.Receipt{}, messaging.ErrInvalid
	}
	room, err := c.channel(ctx, in.Conversation)
	if err != nil {
		return messaging.Receipt{}, err
	}
	path, err := c.messageWritePath(ctx, room, in.Root)
	if err != nil {
		return messaging.Receipt{}, err
	}
	body, err := json.Marshal(struct {
		Content string `json:"content"`
		Request string `json:"client_message_id"`
	}{in.Text, in.RequestID})
	if err != nil {
		return messaging.Receipt{}, err
	}
	raw, err := c.http.Call(ctx, http.MethodPost, path, body)
	if err != nil {
		return messaging.Receipt{}, err
	}
	var sent participantMessage
	if json.Unmarshal(raw, &sent) != nil || sent.ID == "" || sent.Channel != room.ID || sent.Author != c.user {
		return messaging.Receipt{}, messaging.ErrDeliveryUnknown
	}
	return messaging.Receipt{ID: sent.ID, Conversation: in.Conversation}, nil
}

func (c *Participant) messageWritePath(ctx context.Context, room participantRoom, root string) (string, error) {
	path := "/channels/" + url.PathEscape(immutableChannelRef(room.ID)) + "/messages"
	if root != "" {
		parent, err := c.message(ctx, room, root)
		if err != nil {
			return "", err
		}
		if parent.Deleted {
			return "", messaging.ErrUnavailable
		}
		path = "/messages/" + url.PathEscape(root) + "/thread"
	}
	return path, nil
}

func validParticipantSend(in messaging.Send) bool {
	return in.RequestID != "" && strings.TrimSpace(in.Text) != "" && utf8.RuneCountInString(in.Text) <= 40000
}
