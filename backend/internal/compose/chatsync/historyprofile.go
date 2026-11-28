// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HistoryProfile records the installation limits used to judge catch-up support.
func (c LiveChats) HistoryProfile(ctx context.Context, provider string) (*chatwire.HistoryProfile, error) {
	if provider == "slack" && c.Profiles != nil {
		profile, err := c.Profiles(principal.WithActor(ctx, chatLinksPrincipal), provider)
		if err != nil || profile != nil {
			return profile, err
		}
	}
	if provider != "slack" || c.Vault == nil || c.Vault() == nil {
		return nil, nil //nolint:nilnil // History profiles are optional outside a connected Slack installation.
	}
	registry := capture.NewRegistry(c.DB, nil, nil, c.Vault())
	bot, err := registry.LiveBot(principal.WithActor(ctx, chatLinksPrincipal), provider)
	if errors.Is(err, capture.ErrNoConnection) {
		return nil, nil //nolint:nilnil // History profiles are optional outside a connected Slack installation.
	}
	if err != nil {
		return nil, err
	}
	var credential chatwire.BotCredential
	if json.Unmarshal([]byte(bot.Token), &credential) != nil {
		return nil, chatwire.ErrTokenRejected
	}
	return credential.SlackHistory, nil
}
