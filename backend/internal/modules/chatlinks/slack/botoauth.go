// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// AuthorizeBotURL starts installation consent separately from personal Messenger consent.
func (o OAuth) AuthorizeBotURL(state, redirect string) string {
	return "https://slack.com/oauth/v2/authorize?" + url.Values{"client_id": {o.ClientID}, redirectURIParam: {redirect}, "state": {state}, "scope": {strings.Join(BotScopes, ",")}}.Encode()
}

// ExchangeBot accepts only the installation credential from bot consent.
func (o OAuth) ExchangeBot(ctx context.Context, code, redirect string) (Authorization, error) {
	response, err := o.redeem(ctx, url.Values{"code": {code}, "redirect_uri": {redirect}})
	if err != nil {
		return Authorization{}, err
	}
	credential, err := o.credentialFor(response.oauthToken, "bot", BotScopes)
	if err != nil || response.Team.ID == "" || response.BotUser == "" {
		return Authorization{}, messaging.ErrUnauthorized
	}
	return Authorization{Credential: credential, Workspace: response.Team.ID, Member: response.BotUser}, nil
}

// RefreshBot rotates the installation credential.
func (o OAuth) RefreshBot(ctx context.Context, previous messaging.Credential) (messaging.Credential, error) {
	if previous.AppID != o.ClientID || previous.RefreshToken == "" {
		return messaging.Credential{}, messaging.ErrUnauthorized
	}
	response, err := o.redeem(ctx, url.Values{grantTypeParam: {refreshTokenParam}, refreshTokenParam: {previous.RefreshToken}})
	if err != nil {
		return messaging.Credential{}, err
	}
	return o.credentialFor(response.oauthToken, "bot", RequiredBotScopes())
}
