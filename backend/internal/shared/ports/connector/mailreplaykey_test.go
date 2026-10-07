// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "testing"

// The two halves are one rule spelled on two sides of a boundary — capture
// writes the key, activities reads it back as the In-Reply-To of a reply — so
// they are held together here rather than each beside its caller.

func TestTheSharedMessageIDSurvivesBeingScopedToASeat(t *testing.T) {
	const messageID = "CAF=abc123@mail.example.com"
	scoped := SeatScopedMailKey(messageID, "0199c0de-0000-7000-8000-000000000001")

	if scoped == messageID {
		t.Fatal("scoping a key to a seat left it unchanged; two seats would collide on it again")
	}
	if got := SharedMailKey(scoped); got != messageID {
		t.Errorf("the shared key behind %q is %q, want %q — a reply would thread onto nothing", scoped, got, messageID)
	}
}

// A bare key is already shared, and asking is how every reader of a stored key
// works: it cannot know which of the two shapes it holds.
func TestAnUnscopedKeyIsItsOwnSharedKey(t *testing.T) {
	const messageID = "CAF=abc123@mail.example.com"
	if got := SharedMailKey(messageID); got != messageID {
		t.Errorf("SharedMailKey(%q) = %q, want it unchanged", messageID, got)
	}
}

// The marker is chosen so a scoped key can never pass as an identity: ValidMessageID
// refuses a space, so a reader that forgot to take the shared key first starts a
// conversation rather than emitting a header naming somebody's seat.
func TestAScopedKeyIsNeverAValidMessageID(t *testing.T) {
	scoped := SeatScopedMailKey("CAF=abc123@mail.example.com", "0199c0de-0000-7000-8000-000000000001")
	if ValidMessageID(scoped) {
		t.Errorf("%q passes as an RFC822 identity; a seat id would go out on the wire", scoped)
	}
}

// And a sender cannot forge one. The marker is unrepresentable in a genuine
// Message-ID, so no incoming header can arrive already looking scoped and claim
// another seat's shared key.
func TestASenderCannotTypeAKeyThatLooksScoped(t *testing.T) {
	forged := "a" + mailSeatMarker + "b@example.com"
	if ValidMessageID(forged) {
		t.Errorf("%q is accepted as a Message-ID, so a sender could type a scoped-looking key", forged)
	}
}
