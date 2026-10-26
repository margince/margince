// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Only an AI client connected on a colleague's own passport, outside a
// scheduled run, is that colleague's live conversation.
func TestOnlyAConnectedClientOutsideARunIsAConversation(t *testing.T) {
	colleague := ids.NewV7()
	connected := principal.Principal{
		Type: principal.PrincipalAgent, PassportID: ids.NewV7(), ConnectionID: ids.NewV7(), OnBehalfOf: colleague,
	}
	minted := connected
	minted.ConnectionID = ids.UUID{}
	human := principal.Principal{Type: principal.PrincipalHuman, UserID: colleague, OnBehalfOf: colleague}
	run := principal.WithAgentRunID(context.Background(), ids.NewV7())
	for _, tc := range []struct {
		name string
		ctx  context.Context
		p    principal.Principal
		want ids.UUID
	}{
		{"a connected client", context.Background(), connected, colleague},
		{"a scheduled run on that client's passport", run, connected, ids.Nil},
		{"a passport minted without a connection", context.Background(), minted, ids.Nil},
		{"a human", context.Background(), human, ids.Nil},
	} {
		if got := conversationOf(tc.ctx, tc.p); got != tc.want {
			t.Errorf("%s: conversationOf = %s, want %s", tc.name, got, tc.want)
		}
	}
}
