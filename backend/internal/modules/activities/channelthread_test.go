// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAMergedContactIsRepliedToOnTheChatTheThreadNames(t *testing.T) {
	whatsapp := connector.ChannelIdentity{ChannelUserID: "whatsapp/849/c1"}
	zalo := connector.ChannelIdentity{ChannelUserID: "zalo/77/c1"}
	got := namedByThread([]connector.ChannelIdentity{whatsapp, zalo}, "personal_channels:c1:zalo/77/c1")
	if len(got) != 1 || got[0] != zalo {
		t.Fatalf("recipients = %v, want only the Zalo account the thread names", got)
	}
}

func TestAThreadNamingNoAccountLeavesTheChoiceAmbiguous(t *testing.T) {
	both := []connector.ChannelIdentity{{ChannelUserID: "a"}, {ChannelUserID: "b"}}
	if got := namedByThread(both, "telegram:12345"); len(got) != 2 {
		t.Fatalf("recipients = %v, want both kept so the send is refused as ambiguous", got)
	}
}
