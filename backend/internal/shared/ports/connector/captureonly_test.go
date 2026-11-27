// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"errors"
	"testing"
)

func TestAClarificationGoesOnlyToAVerifiedColleagueOnAnyThread(t *testing.T) {
	t.Parallel()
	for _, thread := range []string{"telegram:123:456", "telegram:biz:8800:5001", "whatsapp:biz:109:4915112345678"} {
		if err := AllowChannelSend(PurposeClarification, "telegram", thread, false); !errors.Is(err, ErrClarificationRecipient) {
			t.Errorf("a question to an unverified recipient on %s = %v, want it refused", thread, err)
		}
		if err := AllowChannelSend(PurposeClarification, "telegram", thread, true); err != nil {
			t.Errorf("a question to a verified colleague on %s = %v", thread, err)
		}
	}
}

func TestAReplyIntoACustomerBusinessConversationIsRefusedWhoeverReceivesIt(t *testing.T) {
	t.Parallel()
	var refused *CaptureOnlyConversationError
	if err := AllowChannelSend(PurposeReply, "whatsapp", "whatsapp:biz:109:4915112345678", true); !errors.As(err, &refused) {
		t.Fatalf("reply = %v, want a capture-only refusal", err)
	}
	if err := AllowChannelSend(PurposeReply, "telegram", "telegram:123:456", false); err != nil {
		t.Fatalf("a reply on an ordinary thread = %v", err)
	}
}
