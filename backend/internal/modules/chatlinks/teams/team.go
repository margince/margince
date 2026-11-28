// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

// ForTeam shares app tokens while binding requests to another explicitly approved installation.
func (b *Bot) ForTeam(credential chatwire.BotCredential) (*Bot, error) {
	if credential.Workspace != b.graph.tenant || credential.Credential.AppID != b.app || credential.Member == "" || credential.NativeTeam == "" || credential.BotMember == "" {
		return nil, messaging.ErrInvalid
	}
	if err := CheckServiceURL(credential.ServiceURL); err != nil {
		return nil, err
	}
	next := *b
	next.installationVerified = false
	graph := *b.graph
	graph.botTeam = credential.Member
	next.graph, next.team, next.nativeTeam, next.member = &graph, credential.Member, credential.NativeTeam, credential.BotMember
	next.connector.Base = credential.ServiceURL
	return &next, nil
}
