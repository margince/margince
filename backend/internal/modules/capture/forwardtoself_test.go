// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A Message-ID is the sender's to type. Two senders' originals that share one
// are two originals, and a seat's forwards of them are two activities.
func TestAForwardIsKnownByItsOriginalsSenderAndMessageID(t *testing.T) {
	seat := ids.NewV7()
	keyOf := func(from, messageID string) string {
		return forwardKey(seat, ActivityFields{forwardedFrom: &connector.ForwardedOriginal{From: from, MessageID: messageID}})
	}
	first := keyOf("bea@prospect.example", "one@x")
	if first == "" || first != keyOf("bea@prospect.example", "one@x") {
		t.Error("one original forwarded twice has no stable key, want one")
	}
	if keyOf("bea@prospect.example", "one@x") == keyOf("mallory@elsewhere.example", "one@x") {
		t.Error("two senders' originals sharing a Message-ID share a key, want two")
	}
	if forwardKey(ids.Nil, ActivityFields{forwardedFrom: &connector.ForwardedOriginal{From: "bea@prospect.example"}}) != "" {
		t.Error("a forward with no seat has a key, want none")
	}
}
