// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TeamsBotCheck verifies the installed Teams identity before channel use.
type TeamsBotCheck struct {
	HTTP messaging.Doer
	App  BotApp
}

// NeedsAddress reports whether installation consent requires a provider base address.
func (TeamsBotCheck) NeedsAddress() bool { return false }

// FixedIdentity requires reconnects to preserve the established installation identity.
func (TeamsBotCheck) FixedIdentity() bool { return true }

// ValidateToken rejects unusable installation credential payloads before confirmation.
func (TeamsBotCheck) ValidateToken(token string) error {
	var credential chatwire.BotCredential
	if json.Unmarshal([]byte(token), &credential) != nil || credential.Credential.AppID == "" || credential.NativeTeam == "" || credential.BotMember == "" {
		return capture.ErrChannelTokenRejected
	}
	if _, err := ids.Parse(credential.Workspace); err != nil {
		return capture.ErrChannelTokenRejected
	}
	if _, err := ids.Parse(credential.Member); err != nil {
		return capture.ErrChannelTokenRejected
	}
	if err := teams.CheckServiceURL(credential.ServiceURL); err != nil {
		return capture.ErrChannelTokenRejected
	}
	return nil
}

// Confirm checks installation identity before the registry stores its credential.
func (c TeamsBotCheck) Confirm(ctx context.Context, _ string, token string) (capture.ChannelBot, error) {
	if err := c.ValidateToken(token); err != nil {
		return capture.ChannelBot{}, err
	}
	if c.App == nil {
		return capture.ChannelBot{}, capture.ErrChannelTokenRejected
	}
	app, err := c.App(ctx, chatlinks.ProviderTeams)
	if err != nil {
		return capture.ChannelBot{}, err
	}
	var credential chatwire.BotCredential
	if json.Unmarshal([]byte(token), &credential) != nil {
		return capture.ChannelBot{}, capture.ErrChannelTokenRejected
	}
	native, err := TeamsBot(ctx, c.HTTP, app, credential)
	if errors.Is(err, messaging.ErrUnauthorized) {
		return capture.ChannelBot{}, capture.ErrChannelTokenRejected
	}
	if err != nil {
		return capture.ChannelBot{}, capture.ErrChannelProviderUnreachable
	}
	bot, err := native.Me(ctx)
	if errors.Is(err, chatwire.ErrTokenRejected) || errors.Is(err, chatwire.ErrNotMember) {
		return capture.ChannelBot{}, capture.ErrChannelTokenRejected
	}
	if err != nil {
		return capture.ChannelBot{}, capture.ErrChannelProviderUnreachable
	}
	return capture.ChannelBot{ID: credential.Workspace + "/" + credential.Member, Label: bot.Name}, nil
}
