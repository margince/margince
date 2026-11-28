// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// BotScopes limits installation consent to channel operations.
var BotScopes = []string{"TeamSettings.Read.Group", "TeamMember.Read.Group", "ChannelSettings.Read.Group", "ChannelMember.Read.Group", "ChannelMessage.Read.Group", "TeamsAppInstallation.Read.Group"}

// Bot performs channel synchronization through an installed application's credentials.
type Bot struct {
	installationVerified bool
	references           ThreadReference
	graph                *Client
	connector            messaging.HTTP
	app                  string
	team                 string
	nativeTeam           string
	member               string
}

// NewBot binds channel operations to the installed application's identity.
func NewBot(httpClient messaging.Doer, graphToken, connectorToken string, credential chatwire.BotCredential) *Bot {
	graph := New(httpClient, graphToken, credential.Workspace, "")
	graph.botTeam = credential.Member
	return &Bot{graph: graph, connector: messaging.HTTP{Client: httpClient, Base: strings.TrimRight(credential.ServiceURL, "/"), Token: connectorToken}, app: credential.Credential.AppID, team: credential.Member, nativeTeam: credential.NativeTeam, member: credential.BotMember}
}

// Me resolves the native installation identity behind the credential.
func (b *Bot) Me(ctx context.Context) (chatwire.Bot, error) {
	raw, err := b.graph.http.Call(ctx, http.MethodGet, "/teams/"+url.PathEscape(b.team), nil)
	if err != nil {
		return chatwire.Bot{}, teamsBotFailure(err)
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var team struct {
		ID       string `json:"id"`
		Name     string `json:"displayName"`
		Archived bool   `json:"isArchived"`
	}
	if json.Unmarshal(raw, &team) != nil || team.ID != b.team || team.Archived {
		return chatwire.Bot{}, chatwire.ErrNotMember
	}
	path := "/teams/" + url.PathEscape(b.team) + "/installedApps?$expand=teamsAppDefinition"
	for range 20 {
		raw, err := b.graph.http.Call(ctx, http.MethodGet, path, nil)
		if err != nil {
			return chatwire.Bot{}, teamsBotFailure(err)
		}
		//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
		var page struct {
			Apps []struct {
				Definition struct {
					App string `json:"azureADAppId"`
				} `json:"teamsAppDefinition"`
			} `json:"value"`
			Next string `json:"@odata.nextLink"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return chatwire.Bot{}, chatwire.ErrUnreachable
		}
		for _, app := range page.Apps {
			if app.Definition.App == b.app {
				b.installationVerified = true
				return chatwire.Bot{ID: b.member, TeamID: b.graph.tenant, Name: team.Name}, nil
			}
		}
		if page.Next == "" {
			return chatwire.Bot{}, chatwire.ErrNotMember
		}
		next, err := nextPath(page.Next)
		expected := "/teams/" + url.PathEscape(b.team) + "/installedApps?"
		if err != nil || !strings.HasPrefix(next, expected) || next == path {
			return chatwire.Bot{}, chatwire.ErrUnreachable
		}
		path = next
	}
	return chatwire.Bot{}, chatwire.ErrUnreachable
}

// Channel retains native channel identity independently of its mutable display name.
func (b *Bot) Channel(ctx context.Context, id string) (chatwire.Channel, error) {
	if err := b.ensureInstallation(ctx); err != nil {
		return chatwire.Channel{}, err
	}
	path, ok := channelPath(id)
	team, _, _ := strings.Cut(id, "/")
	if !ok || team != b.team {
		return chatwire.Channel{}, chatwire.ErrNotMember
	}
	raw, err := b.graph.http.Call(ctx, http.MethodGet, path, nil)
	if err != nil {
		return chatwire.Channel{}, teamsBotFailure(err)
	}
	//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
	var channel struct {
		ID         string `json:"id"`
		Name       string `json:"displayName"`
		Membership string `json:"membershipType"`
		Archived   bool   `json:"isArchived"`
	}
	if json.Unmarshal(raw, &channel) != nil || id != team+"/"+channel.ID {
		return chatwire.Channel{}, chatwire.ErrUnreachable
	}
	if channel.Membership != standardChannelKind && channel.Membership != privateChannelKind && channel.Membership != sharedChannelKind {
		return chatwire.Channel{}, messaging.ErrUnsupported
	}
	posting := true
	if channel.Membership != standardChannelKind {
		if _, err := b.graph.http.Call(ctx, http.MethodGet, path+"/messages?$top=1", nil); err != nil {
			return chatwire.Channel{}, teamsBotFailure(err)
		}
	}
	return chatwire.Channel{Slug: id, Name: channel.Name, IsPrivate: true, BotIsMember: true, IsArchived: channel.Archived, PostingAllowed: &posting}, nil
}

// Channels lists native rooms available to the current credential.
func (b *Bot) Channels(ctx context.Context) ([]chatwire.Channel, error) {
	if err := b.ensureInstallation(ctx); err != nil {
		return nil, err
	}
	expected := "/teams/" + url.PathEscape(b.team) + "/channels"
	path := expected + "?$select=id,displayName,membershipType,isArchived"
	out := []chatwire.Channel{}
	for range 100 {
		raw, err := b.graph.http.Call(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, teamsBotFailure(err)
		}
		//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
		var page struct {
			Channels []struct {
				ID         string `json:"id"`
				Name       string `json:"displayName"`
				Membership string `json:"membershipType"`
				Archived   bool   `json:"isArchived"`
			} `json:"value"`
			Next string `json:"@odata.nextLink"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return nil, chatwire.ErrUnreachable
		}
		for _, channel := range page.Channels {
			if channel.ID == "" || channel.Archived || channel.Membership != "private" && channel.Membership != standardChannelKind && channel.Membership != sharedChannelKind {
				continue
			}
			if channel.Membership != standardChannelKind {
				verified, err := b.Channel(ctx, b.team+"/"+channel.ID)
				if errors.Is(err, chatwire.ErrNotMember) {
					continue
				}
				if err != nil {
					return nil, err
				}
				out = append(out, verified)
			} else {
				posting := true
				out = append(out, chatwire.Channel{Slug: b.team + "/" + channel.ID, Name: channel.Name, IsPrivate: true, BotIsMember: true, PostingAllowed: &posting})
			}
		}
		if page.Next == "" {
			return out, nil
		}
		next, err := nextPath(page.Next)
		if err != nil || !strings.HasPrefix(next, expected+"?") || next == path {
			return nil, chatwire.ErrUnreachable
		}
		path = next
	}
	return nil, chatwire.ErrUnreachable
}

// Audience resolves effective readers for comparison with the record team.
func (b *Bot) Audience(ctx context.Context, id string) (messaging.Conversation, error) {
	if _, err := b.Channel(ctx, id); err != nil {
		return messaging.Conversation{}, err
	}
	audience, err := b.graph.Conversation(ctx, id)
	if err != nil {
		return messaging.Conversation{}, teamsBotFailure(err)
	}
	if !audience.AudienceKnown || audience.External || len(audience.Members) == 0 {
		return messaging.Conversation{}, chatwire.ErrAudienceNeedsReview
	}
	for _, member := range audience.Members {
		if !member.Active {
			return messaging.Conversation{}, chatwire.ErrAudienceNeedsReview
		}
	}
	return audience, nil
}

// CreateChannel creates an internal destination when the provider permits it.
func (b *Bot) CreateChannel(context.Context, string, string) (chatwire.Channel, error) {
	return chatwire.Channel{}, messaging.ErrUnsupported
}

// AddMembers reports native membership changes separately from unresolved addresses.
func (b *Bot) AddMembers(context.Context, string, []string) (chatwire.MemberResult, error) {
	return chatwire.MemberResult{}, messaging.ErrUnsupported
}

// History serves the legacy sequence interface where the provider supports it.
func (b *Bot) History(context.Context, string, int64, int) (chatwire.HistoryPage, error) {
	return chatwire.HistoryPage{}, messaging.ErrUnsupported
}

func teamsBotFailure(err error) error {
	if errors.Is(err, messaging.ErrForbidden) {
		return errors.Join(chatwire.ErrNotMember, err)
	}
	if errors.Is(err, messaging.ErrUnauthorized) {
		return errors.Join(chatwire.ErrTokenRejected, err)
	}
	return err
}

func (b *Bot) ensureInstallation(ctx context.Context) error {
	if b.installationVerified {
		return nil
	}
	_, err := b.Me(ctx)
	return err
}
