// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

func TestARemainingAskQuotesTheirWordsNotTheSignatureOrAFragment(t *testing.T) {
	t.Parallel()
	body := "Please send the report.\n\n-- \nKind regards\nA sender"
	for _, quote := range []string{"Please", "Please .", "Kind regards", "A sender", "send the quote"} {
		if RequestRemainderGrounded(body, quote) {
			t.Errorf("%q is not an ask of theirs and was admitted", quote)
		}
	}
	if !RequestRemainderGrounded(body, "send the report") {
		t.Fatal("their own ask was refused")
	}
	if RequestRemainderGrounded("-- \nSend the report", "Send the report") {
		t.Fatal("a message that is only a signature supplied an ask")
	}
}

func TestARemainingAskIsNeverTakenFromTheHistoryQuotedBelowTheirWords(t *testing.T) {
	t.Parallel()
	body := "Thanks for the feedback.\n\nOn Monday someone wrote:\n> Which prospects did you speak to?"
	if RequestRemainderGrounded(body, "Which prospects did you speak to?") {
		t.Fatal("our own question, quoted below their reply, became an ask of theirs")
	}
}

// A greeting or a thank-you asks for nothing. A remainder that quotes only one
// of them names no ask, however many words it carries.
func TestARemainingAskIsNotTheirGreetingOrTheirThanks(t *testing.T) {
	t.Parallel()
	body := "Good morning Anna,\n\nPlease send the signed contract.\n\nThanks again,\nBob"
	for _, quote := range []string{"Good morning Anna", "Thanks again", "Good morning"} {
		if RequestRemainderGrounded(body, quote) {
			t.Errorf("%q is a salutation, not an ask, and was admitted", quote)
		}
	}
	if !RequestRemainderGrounded(body, "send the signed contract") {
		t.Fatal("their own ask was refused")
	}
}

// A sign-off whose name stands under a blank line grounds nothing either, and
// a remainder spelled with decomposed accents still quotes the message.
func TestARemainingAskSkipsASpacedSignOffAndReadsAccentsAlike(t *testing.T) {
	t.Parallel()
	body := "Please send the café menu.\n\nThanks again\n\nBob"
	if RequestRemainderGrounded(body, "Thanks again") {
		t.Error("a sign-off above its name was admitted as an ask")
	}
	if !RequestRemainderGrounded(body, "send the cafe\u0301 menu") {
		t.Error("their ask, with its accent decomposed, was refused")
	}
}
