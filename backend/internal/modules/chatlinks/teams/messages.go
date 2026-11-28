// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/htmltext"
)

//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type message struct {
	Attachments []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"contentType"`
		Content string `json:"content"`
		URL     string `json:"contentUrl"`
	} `json:"attachments"`
	Replies     []json.RawMessage `json:"replies"`
	RepliesNext string            `json:"replies@odata.nextLink"`
	WebURL      string            `json:"webUrl"`
	ID          string            `json:"id"`
	Modified    time.Time         `json:"lastModifiedDateTime"`
	Root        string            `json:"replyToId"`
	At          time.Time         `json:"createdDateTime"`
	Edited      *time.Time        `json:"lastEditedDateTime"`
	Deleted     *time.Time        `json:"deletedDateTime"`
	From        struct {
		User struct {
			ID   string `json:"id"`
			Name string `json:"displayName"`
		} `json:"user"`
		Application struct {
			ID string `json:"id"`
		} `json:"application"`
	} `json:"from"`
	Body struct {
		Type string `json:"contentType"`
		Text string `json:"content"`
	} `json:"body"`
}

func (m message) normalized() (messaging.Message, error) {
	author := m.From.User.ID
	if author == "" {
		author = m.From.Application.ID
	}
	text := m.Body.Text
	if m.Body.Type == "html" {
		var cut bool
		text, cut = htmltext.Render(text)
		if cut {
			return messaging.Message{}, messaging.ErrUnavailable
		}
	}
	incomplete := false
	var visible strings.Builder
	visible.WriteString(text)
	for _, attachment := range m.Attachments {
		name := attachment.Name
		if name == "" {
			name = "Message attachment"
		}
		if attachment.Content != "" && (attachment.Type == "application/vnd.microsoft.card.adaptive" || attachment.Type == "application/vnd.microsoft.card.hero" || attachment.Type == "application/vnd.microsoft.card.thumbnail") {
			card, complete := messaging.RichContent(json.RawMessage(attachment.Content))
			visible.WriteString("\n" + card)
			incomplete = incomplete || !complete
		} else {
			visible.WriteString("\n[" + name + ": content not read]")
			incomplete = true
		}
	}
	text = visible.String()
	if m.Deleted != nil {
		text = ""
	}
	if m.ID == "" {
		return messaging.Message{}, messaging.ErrUnavailable
	}
	return messaging.Message{ReplyCount: len(m.Replies), ContentIncomplete: incomplete, SourceURL: m.WebURL, ID: m.ID, Root: m.Root, Author: author, Text: text, At: m.At, Edited: m.Edited != nil, Deleted: m.Deleted != nil}, nil
}

func messagePath(conversation, root string) (string, error) {
	if path, channel := channelPath(conversation); channel {
		if root != "" {
			return path + "/messages/" + url.PathEscape(root) + "/replies", nil
		}
		return path + "/messages", nil
	}
	if root != "" {
		return "", messaging.ErrUnsupported
	}
	return "/chats/" + url.PathEscape(conversation) + "/messages", nil
}

// Messages reads provider content through the current participant credential.
func (c *Client) Messages(ctx context.Context, conversation, root, cursor string) (messaging.Page, error) {
	if _, err := c.Conversation(ctx, conversation); err != nil {
		return messaging.Page{}, err
	}
	path, err := messagePath(conversation, root)
	if err != nil {
		return messaging.Page{}, err
	}
	path, err = graphPagePath(path, cursor)
	if err == nil && root == "" && cursor == "" && strings.HasPrefix(path, "/teams/") {
		path += "&$expand=replies"
	}
	if err != nil {
		return messaging.Page{}, err
	}
	raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return messaging.Page{}, err
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var page struct {
		Messages []message `json:"value"`
		Next     string    `json:"@odata.nextLink"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return messaging.Page{}, messaging.ErrUnavailable
	}
	out := messaging.Page{Messages: []messaging.Message{}, Next: page.Next}
	if root != "" && cursor == "" {
		parentPath, channel := channelPath(conversation)
		if !channel {
			return messaging.Page{}, messaging.ErrInvalid
		}
		raw, err = c.http.Call(ctx, http.MethodGet, parentPath+"/messages/"+url.PathEscape(root), nil)
		if err != nil {
			return messaging.Page{}, err
		}
		var parent message
		if json.Unmarshal(raw, &parent) != nil {
			return messaging.Page{}, messaging.ErrUnavailable
		}
		normalized, err := parent.normalized()
		if err != nil {
			return messaging.Page{}, err
		}
		out.Messages = append(out.Messages, normalized)
	}
	for _, item := range page.Messages {
		normalized, err := item.normalized()
		if err != nil {
			return messaging.Page{}, err
		}
		out.Messages = append(out.Messages, normalized)
	}
	return out, nil
}

// Send uses the participant's identity for a reviewed personal message.
func (c *Client) Send(ctx context.Context, in messaging.Send) (messaging.Receipt, error) {
	if strings.TrimSpace(in.Text) == "" || len(in.Text) > 90000 || in.RequestID == "" {
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
	path, err := messagePath(in.Conversation, in.Root)
	if err != nil {
		return messaging.Receipt{}, err
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	body, err := json.Marshal(struct {
		Body struct {
			Type    string `json:"contentType"`
			Content string `json:"content"`
		} `json:"body"`
	}{Body: struct {
		Type    string `json:"contentType"`
		Content string `json:"content"`
	}{Type: "text", Content: in.Text}})
	if err != nil {
		return messaging.Receipt{}, messaging.ErrInvalid
	}
	raw, err := c.http.Call(ctx, http.MethodPost, path, body)
	if err != nil {
		return messaging.Receipt{}, err
	}
	var response message
	if json.Unmarshal(raw, &response) != nil || response.ID == "" {
		return messaging.Receipt{}, messaging.ErrDeliveryUnknown
	}
	return messaging.Receipt{ID: response.ID, Conversation: in.Conversation}, nil
}
