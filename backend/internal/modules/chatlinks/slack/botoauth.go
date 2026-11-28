// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package slack

import (
	"context"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// ChannelReaderScopes declares the delegated channel-reading permissions.
var ChannelReaderScopes = []string{channelHistoryScope, groupsReadScope, usersReadScope, usersEmailScope}

// AuthorizeBotURL starts installation consent separately from personal Messenger consent.
func (o OAuth) AuthorizeBotURL(state, redirect string) string {
	return "https://slack.com/oauth/v2/authorize?" + url.Values{"client_id": {o.ClientID}, redirectURIParam: {redirect}, "state": {state}, "scope": {strings.Join(BotScopes, ",")}, "user_scope": {strings.Join(ChannelReaderScopes, ",")}}.Encode()
}

// ExchangeBot keeps installation and channel-reader credentials distinct during consent.
func (o OAuth) ExchangeBot(ctx context.Context, code, redirect string) (Authorization, error) {
	response, err := o.redeem(ctx, url.Values{"code": {code}, "redirect_uri": {redirect}})
	if err != nil {
		return Authorization{}, err
	}
	credential, err := o.credentialFor(response.oauthToken, "bot", BotScopes)
	if err != nil || response.Team.ID == "" || response.BotUser == "" {
		return Authorization{}, messaging.ErrUnauthorized
	}
	reader, err := o.credentialFor(response.User.oauthToken, "user", ChannelReaderScopes)
	if err != nil || response.User.ID == "" {
		return Authorization{}, messaging.ErrUnauthorized
	}
	return Authorization{Reader: &reader, ReaderMember: response.User.ID, Credential: credential, Workspace: response.Team.ID, Member: response.BotUser}, nil
}

// RefreshBot rotates the installation credential independently of the channel reader.
func (o OAuth) RefreshBot(ctx context.Context, previous messaging.Credential) (messaging.Credential, error) {
	if previous.AppID != o.ClientID || previous.RefreshToken == "" {
		return messaging.Credential{}, messaging.ErrUnauthorized
	}
	response, err := o.redeem(ctx, url.Values{grantTypeParam: {refreshTokenParam}, refreshTokenParam: {previous.RefreshToken}})
	if err != nil {
		return messaging.Credential{}, err
	}
	return o.credentialFor(response.oauthToken, "bot", BotScopes)
}

// RefreshChannelReader rotates the read-only channel grant independently of the bot.
func (o OAuth) RefreshChannelReader(ctx context.Context, previous messaging.Credential) (messaging.Credential, error) {
	if previous.AppID != o.ClientID || previous.RefreshToken == "" {
		return messaging.Credential{}, messaging.ErrUnauthorized
	}
	response, err := o.redeem(ctx, url.Values{"grant_type": {refreshTokenParam}, refreshTokenParam: {previous.RefreshToken}})
	if err != nil {
		return messaging.Credential{}, err
	}
	return o.credentialFor(response.oauthToken, "user", ChannelReaderScopes)
}
