// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/modules/webhooks"
)

// SlackBotCheck verifies the installed Slack identity before it is stored.
type SlackBotCheck struct{ HTTP messaging.Doer }

// NeedsAddress reports whether installation consent requires a provider base address.
func (SlackBotCheck) NeedsAddress() bool { return false }

// FixedIdentity requires reconnects to preserve the established installation identity.
func (SlackBotCheck) FixedIdentity() bool { return true }

// ValidateEventsSecret checks that the installation can verify signed event requests.
func (SlackBotCheck) ValidateEventsSecret(secret string) error { return webhooks.CheckSecret(secret) }

// ValidateToken rejects unusable installation credential payloads before confirmation.
func (SlackBotCheck) ValidateToken(token string) error {
	var credential chatwire.BotCredential
	if json.Unmarshal([]byte(token), &credential) != nil || credential.Workspace == "" || credential.Member == "" || credential.Credential.AccessToken == "" || credential.Credential.AppID == "" {
		return capture.ErrChannelTokenRejected
	}
	for _, scope := range slack.RequiredBotScopes() {
		if !slices.Contains(credential.Credential.Scopes, scope) {
			return capture.ErrChannelTokenRejected
		}
	}
	return nil
}

// Confirm checks installation identity before the registry stores its credential.
func (c SlackBotCheck) Confirm(ctx context.Context, _ string, token string) (capture.ChannelBot, error) {
	if err := c.ValidateToken(token); err != nil {
		return capture.ChannelBot{}, err
	}
	var credential chatwire.BotCredential
	if json.Unmarshal([]byte(token), &credential) != nil {
		return capture.ChannelBot{}, capture.ErrChannelTokenRejected
	}
	bot, err := slack.NewBot(c.HTTP, credential.Credential.AccessToken, credential.Workspace, credential.Member).Me(ctx)
	if errors.Is(err, chatwire.ErrTokenRejected) {
		return capture.ChannelBot{}, capture.ErrChannelTokenRejected
	}
	if err != nil {
		return capture.ChannelBot{}, capture.ErrChannelProviderUnreachable
	}
	return capture.ChannelBot{ID: bot.TeamID, Label: bot.Name}, nil
}
