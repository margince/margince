// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// MaxGroupRecipients leaves room for the sender in the Teams creation request.
const MaxGroupRecipients = 199

func channelPath(id string) (string, bool) {
	team, channel, ok := strings.Cut(id, "/")
	if !ok || team == "" || channel == "" || strings.Contains(channel, "/") {
		return "", false
	}
	return "/teams/" + url.PathEscape(team) + "/channels/" + url.PathEscape(channel), true
}

// Conversation carries native room identity and audience-verification state.
func (c *Client) Conversation(ctx context.Context, id string) (messaging.Conversation, error) {
	path, isChannel, err := c.conversationPath(id)
	if err != nil {
		return messaging.Conversation{}, err
	}
	raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return messaging.Conversation{}, err
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var response struct {
		ID         string `json:"id"`
		Name       string `json:"displayName"`
		Topic      string `json:"topic"`
		Kind       string `json:"chatType"`
		Membership string `json:"membershipType"`
	}
	if json.Unmarshal(raw, &response) != nil || response.ID == "" {
		return messaging.Conversation{}, messaging.ErrUnavailable
	}
	out := messaging.Conversation{Access: "participants", ID: id, Name: response.Topic, Kind: "group"}
	membersPath := path + "/members"
	if response.Kind == "oneOnOne" {
		out.Kind = "direct"
	}
	if isChannel {
		out.Kind = "channel"
		out.Access = privateChannelKind
		out.SupportsThreads = true
		out.Name = response.Name
		if response.Membership == "shared" {
			return messaging.Conversation{}, messaging.ErrUnsupported
		}
		if response.Membership == standardChannelKind {
			out.Access = "team"
			team, _, _ := strings.Cut(id, "/")
			membersPath = "/teams/" + url.PathEscape(team) + "/members"
		}
	}
	members, err := c.members(ctx, membersPath)
	if err != nil {
		return messaging.Conversation{}, err
	}
	out.Members = members
	out.AudienceKnown = true
	for _, member := range members {
		out.External = out.External || member.External
	}
	if c.botTeam == "" && !messaging.HasActiveMember(members, c.user) {
		return messaging.Conversation{}, messaging.ErrUnauthorized
	}
	out.Name = messaging.ConversationLabel(out, c.user)
	return out, nil
}

func (c *Client) members(ctx context.Context, path string) ([]messaging.Member, error) {
	out := []messaging.Member{}
	for range 20 {
		raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
		var page struct {
			Members []struct {
				ID     string   `json:"userId"`
				Tenant string   `json:"tenantId"`
				Name   string   `json:"displayName"`
				Email  string   `json:"email"`
				Roles  []string `json:"roles"`
			} `json:"value"`
			Next string `json:"@odata.nextLink"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return nil, messaging.ErrUnavailable
		}
		for _, m := range page.Members {
			if m.ID == "" || m.Tenant == "" {
				return nil, messaging.ErrUnavailable
			}
			out = append(out, messaging.Member{ID: m.ID, Name: m.Name, Email: m.Email, Active: true, External: m.Tenant != c.tenant || slices.Contains(m.Roles, "guest")})
		}
		if page.Next == "" {
			return out, nil
		}
		next, err := nextPath(page.Next)
		if err != nil || next == path {
			return nil, messaging.ErrUnavailable
		}
		path = next
	}
	return nil, messaging.ErrUnavailable
}

//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type groupMember struct {
	Type  string   `json:"@odata.type"`
	Roles []string `json:"roles"`
	User  string   `json:"user@odata.bind"`
}

// OpenPeople opens the explicitly selected native recipient set.
func (c *Client) OpenPeople(ctx context.Context, people []string) (messaging.Conversation, error) {
	if len(people) < 1 || len(people) > MaxGroupRecipients {
		return messaging.Conversation{}, messaging.ErrUnsupported
	}
	if slices.Contains(people, c.user) {
		return messaging.Conversation{}, messaging.ErrInvalid
	}
	unique := map[string]bool{c.user: true}
	members, err := c.groupMembers(ctx, people, unique)
	if err != nil {
		return messaging.Conversation{}, err
	}
	kind := "group"
	if len(people) == 1 {
		kind = "oneOnOne"
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	body, err := json.Marshal(struct {
		Kind    string        `json:"chatType"`
		Members []groupMember `json:"members"`
	}{Kind: kind, Members: members})
	if err != nil {
		return messaging.Conversation{}, messaging.ErrInvalid
	}
	raw, err := c.http.Call(ctx, http.MethodPost, "/chats", body)
	if err != nil {
		return messaging.Conversation{}, err
	}
	var response struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &response) != nil || response.ID == "" {
		return messaging.Conversation{}, messaging.ErrUnavailable
	}
	conversation, err := c.Conversation(ctx, response.ID)
	if err != nil {
		return messaging.Conversation{}, err
	}
	if len(conversation.Members) != len(unique) {
		return messaging.Conversation{}, messaging.ErrInvalid
	}
	for _, m := range conversation.Members {
		if !unique[m.ID] {
			return messaging.Conversation{}, messaging.ErrInvalid
		}
	}
	return conversation, nil
}

func (c *Client) groupMembers(ctx context.Context, people []string, unique map[string]bool) ([]groupMember, error) {
	members := []groupMember{}
	for _, id := range append([]string{c.user}, people...) {
		if id != c.user && unique[id] {
			return nil, messaging.ErrInvalid
		}
		unique[id] = true
		member, err := c.person(ctx, id)
		if err != nil {
			return nil, err
		}
		if !member.Active || member.External {
			return nil, messaging.ErrInvalid
		}
		members = append(members, groupMember{Type: "#microsoft.graph.aadUserConversationMember", Roles: []string{"owner"}, User: "https://graph.microsoft.com/v1.0/users('" + url.PathEscape(id) + "')"})
	}
	return members, nil
}

func (c *Client) conversationPath(id string) (string, bool, error) {
	path := "/chats/" + url.PathEscape(id)
	channelPath, isChannel := channelPath(id)
	if c.botTeam != "" {
		team, _, ok := strings.Cut(id, "/")
		if !ok || !isChannel || team != c.botTeam {
			return "", false, messaging.ErrUnsupported
		}
	}
	if isChannel {
		path = channelPath
	}
	return path, isChannel, nil
}
