// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// MaxGroupRecipients bounds explicit selection for a single private group.
const MaxGroupRecipients = 8

// Conversation carries native room identity and audience-verification state.
func (c *Client) Conversation(ctx context.Context, id string) (messaging.Conversation, error) {
	raw, err := c.call(ctx, http.MethodGet, "conversations.info?"+url.Values{channelKey: {id}}.Encode(), nil)
	if err != nil {
		return messaging.Conversation{}, err
	}
	var response struct {
		Channel channel `json:"channel"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Channel.ID != id || response.Channel.Archived {
		return messaging.Conversation{}, messaging.ErrUnavailable
	}
	out := response.Channel.conversation()
	if c.history != nil {
		supported := c.history.profile != nil && c.history.profile.CanonicalSupported()
		out.SupportsOutcomeDraft = &supported
	}
	out.SourceURL = "https://app.slack.com/client/" + url.PathEscape(c.workspace) + "/" + url.PathEscape(id)
	cursor := ""
	identities := []string{}
	for range 10 {
		raw, err = c.call(ctx, http.MethodGet, "conversations.members?"+url.Values{channelKey: {id}, limitKey: {"100"}, cursorKey: {cursor}}.Encode(), nil)
		if err != nil {
			return messaging.Conversation{}, err
		}
		var page struct {
			Members  []string `json:"members"`
			Metadata struct {
				Next string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return messaging.Conversation{}, messaging.ErrUnavailable
		}
		identities = append(identities, page.Members...)
		if page.Metadata.Next == "" {
			out.AudienceKnown = true
			break
		}
		if cursor == page.Metadata.Next {
			return messaging.Conversation{}, messaging.ErrUnavailable
		}
		cursor = page.Metadata.Next
	}
	if !out.AudienceKnown {
		return messaging.Conversation{}, messaging.ErrUnavailable
	}
	out.Members, err = c.rosterMembers(ctx, identities)
	if err != nil {
		return messaging.Conversation{}, err
	}
	for _, member := range out.Members {
		out.External = out.External || member.External
	}
	if !messaging.HasActiveMember(out.Members, c.user) {
		return messaging.Conversation{}, messaging.ErrUnauthorized
	}
	out.Name = messaging.ConversationLabel(out, c.user)
	return out, nil
}

// OpenPeople opens the explicitly selected native recipient set.
func (c *Client) OpenPeople(ctx context.Context, people []string) (messaging.Conversation, error) {
	if len(people) < 1 || len(people) > MaxGroupRecipients {
		return messaging.Conversation{}, messaging.ErrUnsupported
	}
	unique, err := c.validatePeople(ctx, people)
	if err != nil {
		return messaging.Conversation{}, err
	}
	body, err := json.Marshal(struct {
		Users string `json:"users"`
	}{Users: strings.Join(people, ",")})
	if err != nil {
		return messaging.Conversation{}, messaging.ErrInvalid
	}
	raw, err := c.call(ctx, http.MethodPost, "conversations.open", body)
	if err != nil {
		return messaging.Conversation{}, err
	}
	var response struct {
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Channel.ID == "" {
		return messaging.Conversation{}, messaging.ErrUnavailable
	}
	out, err := c.Conversation(ctx, response.Channel.ID)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if !out.AudienceKnown || len(out.Members) != len(people)+1 {
		return messaging.Conversation{}, messaging.ErrInvalid
	}
	for _, m := range out.Members {
		if m.ID != c.user && !unique[m.ID] {
			return messaging.Conversation{}, messaging.ErrInvalid
		}
	}
	return out, nil
}

func (c *Client) validatePeople(ctx context.Context, people []string) (map[string]bool, error) {
	unique := map[string]bool{}
	for _, id := range people {
		if id == c.user || unique[id] {
			return nil, messaging.ErrInvalid
		}
		unique[id] = true
		member, err := c.member(ctx, id)
		if err != nil {
			return nil, err
		}
		if !member.Active || member.External || member.Bot {
			return nil, messaging.ErrInvalid
		}
	}
	return unique, nil
}
