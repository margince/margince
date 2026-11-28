// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package chatwire defines the provider contract shared by channel sync adapters.
package chatwire

import "github.com/margince/margince/backend/internal/modules/chatlinks/messaging"

// BotCredential binds sealed installation credentials to their native identity.
type BotCredential struct {
	AllowedTeams []string             `json:"allowed_teams,omitempty"`
	SlackHistory *HistoryProfile      `json:"slack_history,omitempty"`
	ServiceURL   string               `json:"service_url,omitempty"`
	NativeTeam   string               `json:"native_team,omitempty"`
	BotMember    string               `json:"bot_member,omitempty"`
	Workspace    string               `json:"workspace"`
	Member       string               `json:"member"`
	Credential   messaging.Credential `json:"credential"`
}
