// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestAReplyIntoABusinessAccountsChatIsRefusedBeforeItIsStaged(t *testing.T) {
	id := ids.NewV7()
	reply := func(threadKey string) error {
		fields := `{"kind":"message","channel_provider":"telegram","thread_key":"` + threadKey + `"}`
		call := NewSendMessageCall(oneRecord(datasource.EntityActivity, id, fields, 1),
			channelProviderSet{"telegram": true}, nil, SendMessageCommand{ActivityID: id, Body: "hello"})
		return call.Guards(context.Background())
	}

	var refused *connector.CaptureOnlyConversationError
	if err := reply("telegram:biz:880001:990001"); !errors.As(err, &refused) {
		t.Fatalf("a reply on a business account's thread is refused with %v, want the capture-only conversation error", err)
	}
	if err := reply("telegram:42:990001"); err != nil {
		t.Fatalf("a reply on the bot's own thread is refused: %v", err)
	}
}
