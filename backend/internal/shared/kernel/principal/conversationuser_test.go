// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package principal

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Only an AI client connected on a colleague's own passport, outside a
// scheduled run, is that colleague's live conversation.
func TestOnlyAConnectedClientOutsideARunIsAConversation(t *testing.T) {
	colleague := ids.NewV7()
	connected := Principal{
		Type: PrincipalAgent, PassportID: ids.NewV7(), ConnectionID: ids.NewV7(), OnBehalfOf: colleague,
	}
	minted := connected
	minted.ConnectionID = ids.UUID{}
	human := Principal{Type: PrincipalHuman, UserID: colleague, OnBehalfOf: colleague}
	run := WithAgentRunID(context.Background(), ids.NewV7())
	for _, tc := range []struct {
		name string
		ctx  context.Context
		p    Principal
		want ids.UUID
	}{
		{"a connected client", context.Background(), connected, colleague},
		{"a scheduled run on that client's passport", run, connected, ids.Nil},
		{"an unattended runner without a persisted run ID", WithUnattended(context.Background()), connected, ids.Nil},
		{"a passport minted without a connection", context.Background(), minted, ids.Nil},
		{"a human", context.Background(), human, ids.Nil},
	} {
		if got := ConversationUser(tc.ctx, tc.p); got != tc.want {
			t.Errorf("%s: ConversationUser = %s, want %s", tc.name, got, tc.want)
		}
	}
}
