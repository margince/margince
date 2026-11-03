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
	} {
		if got := Strip(c.text); got != c.want {
			t.Errorf("%s: Strip = %q, want %q", c.name, got, c.want)
		}
	}
}
