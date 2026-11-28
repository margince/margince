// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BotApp resolves installation credentials at the composition boundary.
type BotApp func(context.Context, string) (capture.ConnectorApp, error)

func (c LiveChats) openSlack(ctx context.Context, registry *capture.Registry) (*slack.Bot, error) {
	if c.App == nil {
		return nil, messaging.ErrUnsupported
	}
	app, err := c.App(ctx, chatlinks.ProviderSlack)
	if err != nil {
		return nil, err
	}
	clock := c.Now
	if clock == nil {
		clock = time.Now
	}
	bot, err := registry.LiveBotUsing(principal.WithActor(ctx, chatLinksPrincipal), chatlinks.ProviderSlack, func(ctx context.Context, raw []byte) ([]byte, error) {
		credential, err := readSlackCredential(raw, app.ClientID)
		if err != nil {
			return nil, err
		}

		flow := slack.OAuth{HTTP: c.HTTP, ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Now: clock}
		fresh, err := refreshSlackToken(ctx, credential.Credential, flow.RefreshBot, clock())
		if err != nil {
			return nil, err
		}
		credential.Credential = fresh

		return json.Marshal(credential)
	})
	if err != nil {
		return nil, err
	}
	credential, err := readSlackCredential([]byte(bot.Token), app.ClientID)
	if err != nil || credential.Workspace != bot.TeamID {
		return nil, chatwire.ErrTokenRejected
	}
	profile, err := c.HistoryProfile(ctx, chatlinks.ProviderSlack)
	if err != nil {
		return nil, err
	}
	native := slack.NewBot(c.HTTP, credential.Credential.AccessToken, credential.Workspace, credential.Member).WithHistoryProfile(profile, credential.Credential.AppID)
	if _, err := native.Me(ctx); err != nil {
		return nil, err
	}
	return native, nil
}

func (c LiveChats) openTeams(ctx context.Context, bot capture.LiveBot) (*teamChats, string, error) {
	if c.App == nil || c.TeamThreads == nil {
		return nil, "", messaging.ErrUnsupported
	}
	app, err := c.App(ctx, chatlinks.ProviderTeams)
	if err != nil {
		return nil, "", err
	}
	var credential chatwire.BotCredential
	if json.Unmarshal([]byte(bot.Token), &credential) != nil || credential.Credential.AppID != app.ClientID || credential.Workspace != app.Tenant || bot.TeamID != credential.Workspace+"/"+credential.Member {
		return nil, "", chatwire.ErrTokenRejected
	}
	selected := slices.Clone(credential.AllowedTeams)
	if len(selected) == 0 {
		selected = []string{credential.Member}
	}
	if len(selected) > 100 {
		return nil, "", messaging.ErrInvalid
	}
	installations, err := c.TeamThreads.TeamInstallations(principal.WithActor(ctx, chatLinksPrincipal), app.ClientID, app.Tenant)
	if err != nil {
		return nil, "", err
	}
	router, err := c.approvedTeams(ctx, app, selected, installations)
	return router, credential.Workspace, err
}

func (c LiveChats) approvedTeams(ctx context.Context, app capture.ConnectorApp, selected []string, installations []chatlinks.TeamInstallation) (*teamChats, error) {
	router := &teamChats{bots: map[string]*teams.Bot{}}
	for _, installation := range installations {
		if !slices.Contains(selected, installation.Team) || !installation.Installed {
			continue
		}
		credential := chatwire.BotCredential{Workspace: app.Tenant, Member: installation.Team, NativeTeam: installation.NativeTeam, BotMember: installation.BotMember, ServiceURL: installation.ServiceURL, Credential: messaging.Credential{AppID: app.ClientID}}
		var bot *teams.Bot
		var err error
		if router.Bot == nil {
			bot, err = TeamsBot(ctx, c.HTTP, app, credential)
		} else {
			bot, err = router.ForTeam(credential)
		}
		if err != nil {
			return nil, err
		}
		bot.WithThreadReferences(func(readCtx context.Context, channel, root string) (teams.ConversationReference, error) {
			return c.TeamThreads.TeamThread(principal.WithActor(readCtx, chatLinksPrincipal), app.ClientID, app.Tenant, channel, root)
		})
		if router.Bot == nil {
			router.Bot = bot
		}
		router.bots[installation.Team] = bot
	}
	if router.Bot == nil {
		return nil, chatwire.ErrNotMember
	}
	return router, nil
}

// TeamsBot opens distinct Graph-read and Bot Framework-delivery credentials.
func TeamsBot(ctx context.Context, httpClient messaging.Doer, app capture.ConnectorApp, credential chatwire.BotCredential) (*teams.Bot, error) {
	if err := teams.CheckServiceURL(credential.ServiceURL); err != nil {
		return nil, err
	}
	if _, err := ids.Parse(app.Tenant); err != nil || app.ClientID == "" || app.ClientSecretRef == "" || credential.Workspace != app.Tenant || credential.Credential.AppID != app.ClientID {
		return nil, messaging.ErrUnauthorized
	}
	client, ok := httpClient.(*http.Client)
	if !ok {
		client = &http.Client{Transport: botTransport{httpClient}, Timeout: callTimeout}
	}
	flow := oauthflow.New(oauthflow.Config{Provider: "teams", ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, TokenURL: "https://login.microsoftonline.com/" + url.PathEscape(app.Tenant) + "/oauth2/v2.0/token", HTTPClient: client, AuthRejected: messaging.ErrUnauthorized, Unreachable: messaging.ErrUnavailable})
	graph, err := flow.ClientCredentials(ctx, "https://graph.microsoft.com/.default")
	if err != nil {
		return nil, err
	}
	connector, err := flow.ClientCredentials(ctx, "https://api.botframework.com/.default")
	if err != nil {
		return nil, err
	}
	return teams.NewBot(httpClient, graph.AccessToken, connector.AccessToken, credential), nil
}

type botTransport struct{ messaging.Doer }

func (t botTransport) RoundTrip(r *http.Request) (*http.Response, error) { return t.Do(r) }

func readSlackCredential(raw []byte, app string) (chatwire.BotCredential, error) {
	var credential chatwire.BotCredential
	if json.Unmarshal(raw, &credential) != nil || credential.Workspace == "" || credential.Member == "" || credential.Credential.AccessToken == "" || credential.Credential.AppID != app {
		return chatwire.BotCredential{}, chatwire.ErrTokenRejected
	}
	return credential, nil
}

// ConnectSlack stores the reviewed native installation through the channel owner.
func ConnectSlack(ctx context.Context, profile *chatwire.HistoryProfile, granted slack.Authorization, signing string, connect func(context.Context, capture.ConnectRequest) (capture.ChannelConnection, error)) error {
	raw, err := json.Marshal(chatwire.BotCredential{SlackHistory: profile, Workspace: granted.Workspace, Member: granted.Member, Credential: granted.Credential})
	if err != nil {
		return err
	}
	if _, err := connect(ctx, capture.ConnectRequest{Provider: chatlinks.ProviderSlack, BotToken: string(raw), EventsSecret: signing}); err != nil {
		return err
	}
	return nil
}
