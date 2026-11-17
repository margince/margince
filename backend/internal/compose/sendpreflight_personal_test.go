// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A reply on a member's own chat is theirs to send while their personal
// channels are on. No workspace bot is ever bound to this provider, so asking
// the bot table is what refused every such reply.
func TestAPersonalChannelReplyIsSendableWhileTheMembersChannelsAreOn(t *testing.T) {
	member := ids.NewV7()
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:x", UserID: member})
	for name, on := range map[string]bool{"on": true, "off": false} {
		t.Run(name, func(t *testing.T) {
			var asked ids.UUID
			grants := &stubGrants{channelBound: true}
			authority := mailboxAuthority{
				grants:            grants,
				mailAppConfigured: func(context.Context, string) (bool, error) { return false, nil },
				personalChannelOn: func(_ context.Context, user ids.UUID) (bool, error) { asked = user; return on, nil },
			}
			got, err := authority.SendCapable(human, personalchannels.Provider)
			if err != nil || got != on {
				t.Fatalf("SendCapable = %v, %v; want %v", got, err, on)
			}
			if asked != member {
				t.Errorf("asked about %v, want the sender %v", asked, member)
			}
			if grants.channelCalls != 0 {
				t.Errorf("the workspace bot table was read for a member's own channel")
			}
		})
	}
}

func TestAPersonalChannelReplyIsRefusedWhereNoMemberSends(t *testing.T) {
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:x", UserID: ids.NewV7()})
	authority := mailboxAuthority{grants: &stubGrants{channelBound: true}, mailAppConfigured: func(context.Context, string) (bool, error) { return false, nil }}
	if got, err := authority.SendCapable(human, personalchannels.Provider); got || err != nil {
		t.Fatalf("SendCapable = %v, %v; want a refusal with no fault", got, err)
	}
}
