// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package messengeraccounts binds participant credentials to provider clients.
package messengeraccounts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// App keeps personal consent exchanges behind the configured application.
type App struct {
	Workspace string
	Authorize func(string, string) string
	Exchange  func(context.Context, string, string) (chatlinks.MemberGrant, messaging.Credential, error)
	Refresh   func(context.Context, messaging.Credential) (messaging.Credential, error)
	ID        string
}

// SlackApp keeps personal Slack consent separate from channel installation.
func SlackApp(httpClient messaging.Doer, app capture.ConnectorApp, now func() time.Time) App {
	flow := slack.OAuth{HTTP: httpClient, ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Now: now}
	return App{ID: app.ClientID, Authorize: flow.AuthorizeURL, Refresh: flow.Refresh, Exchange: func(ctx context.Context, code, redirect string) (chatlinks.MemberGrant, messaging.Credential, error) {
		authorization, err := flow.Exchange(ctx, code, redirect)
		return chatlinks.MemberGrant{Provider: "slack", Workspace: authorization.Workspace, Member: authorization.Member}, authorization.Credential, err
	}}
}

// TeamsScopes declares personal Graph capabilities separately from channel installation.
var TeamsScopes = []string{"offline_access", "User.Read", "User.Read.All", "Team.ReadBasic.All", "TeamMember.Read.All", "Channel.ReadBasic.All", "ChannelMember.Read.All", "ChannelMessage.Read.All", "ChannelMessage.Send", "Chat.ReadWrite", "Chat.Create", "ChatMember.Read", "ChatMessage.Send"}

// TeamsApp binds personal Graph consent to the configured Entra application.
func TeamsApp(httpClient *http.Client, app capture.ConnectorApp, now func() time.Time) (App, error) {
	if _, err := ids.Parse(app.Tenant); err != nil {
		return App{}, messaging.ErrUnsupported
	}
	base := "https://login.microsoftonline.com/" + url.PathEscape(app.Tenant) + "/oauth2/v2.0/"
	flow := oauthflow.New(oauthflow.Config{Provider: chatlinks.ProviderTeams, ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Scopes: TeamsScopes, AuthURL: base + "authorize", TokenURL: base + "token", ScopeInTokenForms: true, AuthRejected: messaging.ErrUnauthorized, Unreachable: messaging.ErrUnavailable, HTTPClient: httpClient})
	out := App{ID: app.ClientID, Authorize: flow.AuthCodeURL}
	out.Exchange = func(ctx context.Context, code, redirect string) (chatlinks.MemberGrant, messaging.Credential, error) {
		token, err := flow.Exchange(ctx, code, redirect)
		if err != nil {
			return chatlinks.MemberGrant{}, messaging.Credential{}, err
		}
		if !hasTeamsMessengerScopes(token.Scopes) {
			return chatlinks.MemberGrant{}, messaging.Credential{}, messaging.ErrUnauthorized
		}
		identity, err := teams.MessengerIdentity(ctx, httpClient, token.AccessToken, app.Tenant)
		grant := chatlinks.MemberGrant{Provider: "teams", Workspace: identity.Workspace, Member: identity.Member.ID, Address: identity.Member.Email}
		credential := messaging.Credential{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, Scopes: token.Scopes, AppID: app.ClientID, ExpiresAt: now().Add(token.ExpiresIn)}
		return grant, credential, err
	}
	out.Refresh = func(ctx context.Context, previous messaging.Credential) (messaging.Credential, error) {
		if previous.AppID != app.ClientID {
			return messaging.Credential{}, messaging.ErrUnauthorized
		}
		token, err := flow.Refresh(ctx, previous.RefreshToken, previous.Scopes)
		if err != nil {
			return messaging.Credential{}, err
		}
		previous.AccessToken = token.AccessToken
		if token.Rotated != "" {
			previous.RefreshToken = token.Rotated
		}
		previous.ExpiresAt = now().Add(token.ExpiresIn)
		return previous, nil
	}
	return out, nil
}

func hasTeamsMessengerScopes(granted []string) bool {
	for _, scope := range TeamsScopes {
		if scope == "offline_access" {
			continue
		}
		if !slices.ContainsFunc(granted, func(value string) bool { return strings.EqualFold(value, scope) }) {
			return false
		}
	}
	return true
}

// RefreshCredential preserves the personal grant binding during credential rotation.
func RefreshCredential(ctx context.Context, grant chatlinks.MemberGrant, raw []byte, resolve func(context.Context, string) (App, error), now func() time.Time) ([]byte, error) {
	var credential messaging.Credential
	if json.Unmarshal(raw, &credential) != nil || credential.AccessToken == "" {
		return nil, messaging.ErrUnauthorized
	}
	if credential.AppID == "" {
		return raw, nil
	}
	app, err := resolve(ctx, grant.Provider)
	if err != nil {
		return nil, err
	}
	if credential.AppID != app.ID {
		return nil, messaging.ErrUnauthorized
	}
	if credential.ExpiresAt.IsZero() || credential.ExpiresAt.After(now().Add(2*time.Minute)) {
		return raw, nil
	}
	credential, err = app.Refresh(ctx, credential)
	if err != nil {
		return nil, err
	}
	return json.Marshal(credential) // #nosec G117 -- Encoded only for encrypted credential-vault storage.
}

// WorkspaceMatches keeps installation consent within its originally reviewed workspace.
func WorkspaceMatches(expected, installation, granted string) bool {
	if installation != "" {
		if expected != "" && expected != installation {
			return false
		}
		expected = installation
	}
	return expected == "" || granted == expected
}

// SlackInstallationApp pins personal consent to any shared Slack installation.
func SlackInstallationApp(ctx context.Context, httpClient messaging.Doer, app capture.ConnectorApp, now func() time.Time, installedBot func(context.Context, string) (capture.LiveBot, error)) (App, error) {
	account := SlackApp(httpClient, app, now)
	if installedBot != nil {
		installed, err := installedBot(ctx, chatlinks.ProviderSlack)
		if err == nil {
			account.Workspace = installed.TeamID
		} else if !errors.Is(err, capture.ErrNoConnection) {
			return App{}, err
		}
	}
	return account, nil
}
