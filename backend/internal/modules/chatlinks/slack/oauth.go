// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/outbound"
)

// BotScopes declares the shared channel application permissions.
var BotScopes = []string{publicReadScope, publicHistoryScope, groupsReadScope, channelHistoryScope, "groups:write", "chat:write", usersReadScope, usersEmailScope}

// RequiredBotScopes keeps existing private-channel installations usable after public reading is added.
func RequiredBotScopes() []string {
	scopes := []string{}
	for _, scope := range BotScopes {
		if scope != publicReadScope && scope != publicHistoryScope {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

// UserScopes declares personal Slack capabilities separately from the installation grant.
var UserScopes = []string{publicReadScope, "groups:read", "im:read", "mpim:read", "im:write", "mpim:write", publicHistoryScope, "groups:history", "im:history", "mpim:history", "chat:write", "users:read", "users:read.email"}

// OAuth keeps delegated authorization bound to the configured application.
type OAuth struct {
	HTTP         messaging.Doer
	ClientID     string
	ClientSecret string
	Now          func() time.Time
}

// Authorization keeps exchanged credentials bound to the provider identity returned with them.
type Authorization struct {
	Credential messaging.Credential
	Workspace  string
	Member     string
}

// AuthorizeURL starts delegated consent for the configured personal application.
func (o OAuth) AuthorizeURL(state, redirect string) string {
	return "https://slack.com/oauth/v2/authorize?" + url.Values{"client_id": {o.ClientID}, redirectURIParam: {redirect}, "state": {state}, "user_scope": {strings.Join(UserScopes, ",")}}.Encode()
}

type oauthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

type oauthResponse struct {
	BotUser string `json:"bot_user_id"`
	OK      bool   `json:"ok"`
	oauthToken
	Team struct {
		ID string `json:"id"`
	} `json:"team"`
	User struct {
		ID string `json:"id"`
		oauthToken
	} `json:"authed_user"`
}

func (o OAuth) redeem(ctx context.Context, form url.Values) (oauthResponse, error) {
	form.Set("client_id", o.ClientID)
	form.Set("client_secret", o.ClientSecret)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/oauth.v2.access", strings.NewReader(form.Encode()))
	if err != nil {
		return oauthResponse{}, messaging.ErrInvalid
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", outbound.ChatHeader)
	response, err := o.HTTP.Do(request)
	if err != nil {
		return oauthResponse{}, messaging.ErrUnavailable
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return oauthResponse{}, messaging.ErrUnavailable
	}
	var out oauthResponse
	if response.StatusCode != http.StatusOK || json.Unmarshal(raw, &out) != nil || !out.OK {
		return oauthResponse{}, messaging.ErrUnauthorized
	}
	return out, nil
}

func (o OAuth) credential(token oauthToken) (messaging.Credential, error) {
	return o.credentialFor(token, "user", UserScopes)
}

func (o OAuth) credentialFor(token oauthToken, tokenType string, required []string) (messaging.Credential, error) {
	if token.AccessToken == "" || token.TokenType != tokenType {
		return messaging.Credential{}, messaging.ErrUnauthorized
	}
	scopes := strings.Split(token.Scope, ",")
	for _, scope := range required {
		if !slices.Contains(scopes, scope) {
			return messaging.Credential{}, messaging.ErrUnauthorized
		}
	}
	out := messaging.Credential{AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, Scopes: scopes, AppID: o.ClientID}
	if token.ExpiresIn > 0 {
		if token.RefreshToken == "" || token.ExpiresIn > 86400 {
			return messaging.Credential{}, messaging.ErrUnauthorized
		}
		out.ExpiresAt = o.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	return out, nil
}

// Exchange binds an authorization-code response to the configured personal application.
func (o OAuth) Exchange(ctx context.Context, code, redirect string) (Authorization, error) {
	response, err := o.redeem(ctx, url.Values{"code": {code}, "redirect_uri": {redirect}})
	if err != nil {
		return Authorization{}, err
	}
	credential, err := o.credential(response.User.oauthToken)
	if err != nil || response.Team.ID == "" || response.User.ID == "" {
		return Authorization{}, messaging.ErrUnauthorized
	}
	return Authorization{Credential: credential, Workspace: response.Team.ID, Member: response.User.ID}, nil
}

// Refresh exchanges a refresh token using the configured application.
func (o OAuth) Refresh(ctx context.Context, previous messaging.Credential) (messaging.Credential, error) {
	if previous.AppID != o.ClientID || previous.RefreshToken == "" {
		return messaging.Credential{}, messaging.ErrUnauthorized
	}
	response, err := o.redeem(ctx, url.Values{grantTypeParam: {refreshTokenParam}, refreshTokenParam: {previous.RefreshToken}})
	if err != nil {
		return messaging.Credential{}, err
	}
	return o.credential(response.oauthToken)
}
