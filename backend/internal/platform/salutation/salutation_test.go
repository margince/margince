// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import "testing"

// Strip keeps what the sender says between the greeting and the sign-off, and
// never takes an ask for either.
func TestStripLeavesTheMessageBetweenTheGreetingAndTheSignOff(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, text, want string }{
		{"a greeting and a sign-off", "Good morning Anna,\nPlease send the invoice.\nThanks again,\nBob", "Please send the invoice."},
		{"a German sign-off", "Hallo Frau Weber,\nbitte senden Sie das Angebot.\nMit freundlichen Grüßen", "bitte senden Sie das Angebot."},
		{"an ask that opens on thanks", "Thanks, could you send the invoice?", "Thanks, could you send the invoice?"},
		{"a greeting run on into the message", "Hi Marcus I wanted to ask about the price", "Hi Marcus I wanted to ask about the price"},
		{"no greeting and no sign-off", "Send the report.", "Send the report."},
		{"a name under a blank line", "Please send the report.\n\nThanks again\n\nBob", "Please send the report."},
		{"a name and a company", "Please send it.\nKind regards,\nJane Smith\nAcme Corp", "Please send it."},
		{"sincerely", "Please send it.\nSincerely,\nBob", "Please send it."},
		{"a Vietnamese greeting and sign-off", "Kính gửi anh Nam,\nXin gửi báo giá.\nTrân trọng", "Xin gửi báo giá."},
		{"a colon after the opener", "Kính gửi: Anh Nam,\nXin gửi báo giá.", "Xin gửi báo giá."},
		{"a full stop after the name", "Hi Bob. How are you?", "How are you?"},
		{"a comma before a request", "Good morning, could you send the contract?", "could you send the contract?"},
		{"a title kept before its name", "Hallo Hr. Weber,\nbitte das Angebot.", "bitte das Angebot."},
		{"a message that is only a sign-off", "Thanks, Send It", "Thanks, Send It"},
	} {
		if got := Strip(c.text); got != c.want {
			t.Errorf("%s: Strip = %q, want %q", c.name, got, c.want)
		}
	}
}

// A word boundary reads the whole character on either side, so a letter
// written in more than one byte is never a boundary.
func TestAWordBoundaryReadsWholeCharacters(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		text string
		at   int
		want bool
	}{
		{"Hi👋", 2, true},
		{"aü", 1, false},
		{"üb", 1, false},
		{"a b", 1, true},
	} {
		if got := WordBoundary(c.text, c.at); got != c.want {
			t.Errorf("WordBoundary(%q, %d) = %v, want %v", c.text, c.at, got, c.want)
		}
	}
}
