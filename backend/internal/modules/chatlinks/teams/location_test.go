// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func TestMessageLocationPinsTeamChannelAndTenant(t *testing.T) {
	bot := &Bot{team: "team-one", graph: &Client{tenant: "tenant-one"}}
	link := "https://teams.microsoft.com/l/message/19%3Aroom/reply?groupId=team-one&tenantId=tenant-one&parentMessageId=root"
	location, err := bot.LocateMessage(context.Background(), "team-one/19:room", link)
	if err != nil || location.Message != "reply" || location.Root != "root" {
		t.Fatalf("selection lost: %+v %v", location, err)
	}
	for _, reference := range []string{"https://teams.microsoft.com/l/message/other/reply", "https://teams.microsoft.com/l/message/19%3Aroom/reply?tenantId=tenant-other", "https://teams.microsoft.com/l/message/19%3Aroom/reply?groupId=other", "https://teams.microsoft.com.attacker.test/l/message/19%3Aroom/reply", "../reply"} {
		if _, err := bot.LocateMessage(context.Background(), "team-one/19:room", reference); !errors.Is(err, messaging.ErrInvalid) {
			t.Errorf("accepted %q: %v", reference, err)
		}
	}
}
