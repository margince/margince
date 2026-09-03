// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import "testing"

// A controller message answers to authorization and the ladder, and to nothing
// that is about a human. Asking the user gates about it would refuse every one
// for the absence of a seat it was never meant to have.
func TestAControllerSendAnswersToNoUserGate(t *testing.T) {
	got := profileFor(Delivery{SenderKind: SenderController})
	if got != (senderProfile{}) {
		t.Errorf("profileFor(controller) = %+v, want every user gate off", got)
	}
}

// And the converse, or the test above would pass with the whole chain disabled.
func TestAUserSendAnswersToEveryUserGate(t *testing.T) {
	got := profileFor(Delivery{SenderKind: SenderUser})
	want := senderProfile{requiresSendScope: true, requiresLiveSeat: true, carriesFiles: true, paced: true}
	if got != want {
		t.Errorf("profileFor(user) = %+v, want %+v", got, want)
	}
}

// The profile is read off the ROW, not off a flag a caller passed, so a
// delivery cannot claim to be something it is not. An unset kind is a user
// send: that is what every row in the table was before this column existed.
func TestAnUnsetSenderKindIsAUserSend(t *testing.T) {
	if profileFor(Delivery{}) != profileFor(Delivery{SenderKind: SenderUser}) {
		t.Error("a delivery with no sender kind must be gated as a user send")
	}
}
