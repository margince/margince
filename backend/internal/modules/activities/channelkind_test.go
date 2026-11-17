// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

// Whether a row is a channel conversation is a fact about its kind. Whether a
// reply can leave this installation is a fact about the composed transports.
// This module answers only the first, so a message on a registered but
// uncomposed transport (whatsapp today) still reads as a conversation.
func TestIsChannelKindIsTheKindAndNotTheRegistry(t *testing.T) {
	t.Parallel()
	if !IsChannelKind(KindMessage) {
		t.Errorf("%q is not a channel conversation; the kind is a fact about the row", KindMessage)
	}
	for _, kind := range []string{"email", "note", "task", "call", "meeting", "telegram"} {
		if IsChannelKind(kind) {
			t.Errorf("%q is treated as a channel conversation; only %q is one", kind, KindMessage)
		}
	}
}
