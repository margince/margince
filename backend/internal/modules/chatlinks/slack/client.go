// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package slack implements the attended Slack conversation API.
package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

const (
	groupsReadScope   = "groups:read"
	limitKey          = "limit"
	refreshTokenParam = "refresh_token"
	usersEmailScope   = "users:read.email"
	usersReadScope    = "users:read"
)

const (
	channelHistoryScope  = "groups:history"
	channelKey           = "channel"
	channelKind          = "channel"
	cursorKey            = "cursor"
	errorChannelNotFound = "channel_not_found"
	errorNotInChannel    = "not_in_channel"
	grantTypeParam       = "grant_type"
	redirectURIParam     = "redirect_uri"
	repliesMethod        = "conversations.replies"
)

// Client uses a participant's credential for personal Messenger operations.
type Client struct {
	http      messaging.HTTP
	workspace string
	user      string
	bot       bool
}

// New binds a personal client to its credential's workspace and sender.
func New(httpClient messaging.Doer, token, workspace, user string) *Client {
	return &Client{http: messaging.HTTP{Client: httpClient, Base: "https://slack.com/api", Token: token}, workspace: workspace, user: user}
}

var _ messaging.Client = (*Client)(nil)

type envelope struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error"`
	Metadata struct {
		Next string `json:"next_cursor"`
	} `json:"response_metadata"`
}

func (c *Client) call(ctx context.Context, method, path string, body json.RawMessage) (json.RawMessage, error) {
	raw, err := c.http.Call(ctx, method, "/"+path, body)
	if err != nil {
		return nil, err
	}
	var result envelope
	if json.Unmarshal(raw, &result) != nil {
		if method == http.MethodPost {
			return nil, messaging.ErrDeliveryUnknown
		}
		return nil, messaging.ErrUnavailable
	}
	if result.OK {
		return raw, nil
	}
	switch result.Error {
	case "invalid_auth", "not_authed", "token_revoked", "account_inactive", "missing_scope":
		return nil, messaging.ErrUnauthorized
	case errorChannelNotFound, "not_in_channel", "user_not_found", "users_not_found", "thread_not_found", "message_not_found":
		return nil, &apiRefusal{code: result.Error}
	case "internal_error", "fatal_error":
		return nil, messaging.ErrDeliveryUnknown
	default:
		return nil, messaging.ErrInvalid
	}
}

// Identity binds personal consent to one provider workspace and sender.
func (c *Client) Identity(ctx context.Context) (messaging.Identity, error) {
	raw, err := c.call(ctx, http.MethodGet, "auth.test", nil)
	if err != nil {
		return messaging.Identity{}, err
	}
	var actor struct {
		User string `json:"user_id"`
		Team string `json:"team_id"`
	}
	if json.Unmarshal(raw, &actor) != nil || actor.User == "" || actor.Team == "" || actor.Team != c.workspace || actor.User != c.user {
		return messaging.Identity{}, messaging.ErrUnauthorized
	}
	member, err := c.member(ctx, actor.User)
	return messaging.Identity{Member: member, Workspace: actor.Team}, err
}

type user struct {
	ID          string `json:"id"`
	Team        string `json:"team_id"`
	Name        string `json:"real_name"`
	Deleted     bool   `json:"deleted"`
	Bot         bool   `json:"is_bot"`
	Guest       bool   `json:"is_restricted"`
	SingleGuest bool   `json:"is_ultra_restricted"`
	Stranger    bool   `json:"is_stranger"`
	Profile     struct {
		Email string `json:"email"`
	} `json:"profile"`
}

func (c *Client) person(u user) messaging.Member {
	return messaging.Member{ID: u.ID, Name: u.Name, Email: u.Profile.Email, Bot: u.Bot, Active: !u.Deleted, External: u.Team != c.workspace || u.Guest || u.SingleGuest || u.Stranger}
}

func (c *Client) member(ctx context.Context, id string) (messaging.Member, error) {
	raw, err := c.call(ctx, http.MethodGet, "users.info?"+url.Values{"user": {id}}.Encode(), nil)
	if err != nil {
		return messaging.Member{}, err
	}
	var response struct {
		User user `json:"user"`
	}
	if json.Unmarshal(raw, &response) != nil || response.User.ID != id {
		return messaging.Member{}, messaging.ErrUnavailable
	}
	return c.person(response.User), nil
}

type channel struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Member          bool   `json:"is_member"`
	Archived        bool   `json:"is_archived"`
	Private         bool   `json:"is_private"`
	IM              bool   `json:"is_im"`
	MPIM            bool   `json:"is_mpim"`
	External        bool   `json:"is_ext_shared"`
	PendingExternal bool   `json:"is_pending_ext_shared"`
}

func (c channel) conversation() messaging.Conversation {
	kind := channelKind
	if c.IM {
		kind = "direct"
	} else if c.MPIM {
		kind = "group"
	}
	access := "participants"
	if kind == channelKey {
		access = "workspace"
		if c.Private {
			access = "private"
		}
	}
	return messaging.Conversation{Access: access, ID: c.ID, Name: c.Name, Kind: kind, SupportsThreads: true, External: c.External || c.PendingExternal}
}

// Channels lists native rooms available to the current credential.
func (c *Client) Channels(ctx context.Context, cursor string) ([]messaging.Conversation, string, error) {
	raw, err := c.call(ctx, http.MethodGet, "conversations.list?"+url.Values{limitKey: {"100"}, cursorKey: {cursor}, "exclude_archived": {"true"}, "types": {"public_channel,private_channel"}}.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	var page struct {
		Channels []channel `json:"channels"`
		Metadata struct {
			Next string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return nil, "", messaging.ErrUnavailable
	}
	out := []messaging.Conversation{}
	for _, ch := range page.Channels {
		if ch.Member && !ch.Archived {
			out = append(out, ch.conversation())
		}
	}
	return out, page.Metadata.Next, nil
}

type apiRefusal struct{ code string }

func (e *apiRefusal) Error() string { return "Slack refused access to the requested native resource" }
func (e *apiRefusal) Unwrap() error { return messaging.ErrUnavailable }
