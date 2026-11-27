// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package teams implements participant-authorized Graph conversations.
package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

const (
	httpsScheme         = "https"
	messageKind         = "message"
	privateChannelKind  = "private"
	standardChannelKind = "standard"
)

// Client uses a participant's credential for personal Messenger operations.
type Client struct {
	botTeam string
	http    messaging.HTTP
	tenant  string
	user    string
}

// New binds a personal client to its credential's workspace and sender.
func New(client messaging.Doer, token, tenant, user string) *Client {
	return &Client{http: messaging.HTTP{Client: client, Base: "https://graph.microsoft.com/v1.0", Token: token}, tenant: tenant, user: user}
}

var _ messaging.Client = (*Client)(nil)

//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type person struct {
	ID        string `json:"id"`
	Name      string `json:"displayName"`
	Email     string `json:"mail"`
	Principal string `json:"userPrincipalName"`
	Type      string `json:"userType"`
	Enabled   *bool  `json:"accountEnabled"`
}

func (p person) member() messaging.Member {
	email := p.Email
	if email == "" {
		email = p.Principal
	}
	return messaging.Member{ID: p.ID, Name: p.Name, Email: email, Active: p.Enabled == nil || *p.Enabled, External: p.Type != "Member"}
}

func (c *Client) person(ctx context.Context, id string) (messaging.Member, error) {
	path := "/users/" + url.PathEscape(id) + "?$select=id,displayName,mail,userPrincipalName,userType,accountEnabled"
	if id == c.user {
		path = "/me?$select=id,displayName,mail,userPrincipalName,userType,accountEnabled"
	}
	raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return messaging.Member{}, err
	}
	var p person
	if json.Unmarshal(raw, &p) != nil || p.ID != id {
		return messaging.Member{}, messaging.ErrUnauthorized
	}
	return p.member(), nil
}

// Identity binds personal consent to one provider workspace and sender.
func (c *Client) Identity(ctx context.Context) (messaging.Identity, error) {
	if c.tenant == "" || c.user == "" {
		return messaging.Identity{}, messaging.ErrUnauthorized
	}
	member, err := c.person(ctx, c.user)
	if err != nil || !member.Active || member.External {
		return messaging.Identity{}, messaging.ErrUnauthorized
	}
	return messaging.Identity{Member: member, Workspace: c.tenant}, nil
}

// nextPath accepts only Graph's own versioned pagination links. A cursor cannot
// redirect a bearer token to another host or another Graph API version.
func nextPath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != httpsScheme || u.Host != "graph.microsoft.com" || u.User != nil || u.Fragment != "" || !strings.HasPrefix(u.Path, "/v1.0/") {
		return "", messaging.ErrInvalid
	}
	return strings.TrimPrefix(u.EscapedPath(), "/v1.0") + "?" + u.RawQuery, nil
}

// People searches native identities through the current participant credential.
func (c *Client) People(ctx context.Context, query, cursor string) ([]messaging.Member, string, error) {
	path := "/users?" + url.Values{"$select": {"id,displayName,mail,userPrincipalName,userType,accountEnabled"}, "$top": {"50"}, "$filter": {"startswith(displayName,'" + strings.ReplaceAll(query, "'", "''") + "')"}}.Encode()
	if cursor != "" {
		var err error
		path, err = nextPath(cursor)
		if err != nil || !strings.HasPrefix(path, "/users?") {
			return nil, "", messaging.ErrInvalid
		}
	}
	raw, err := c.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", err
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var response struct {
		People []person `json:"value"`
		Next   string   `json:"@odata.nextLink"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return nil, "", messaging.ErrUnavailable
	}
	out := []messaging.Member{}
	for _, p := range response.People {
		member := p.member()
		if member.Active && !member.External {
			out = append(out, member)
		}
	}
	return out, response.Next, nil
}
