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
		{"a message that is only a sign-off", "Thanks, Send It", ""},
		{"a message that is only a greeting", "Good morning Anna,", ""},
		{"a capitalised ask after a colon", "Hello: Please send the report.", "Please send the report."},
		{"a capitalised ask after a comma", "Good morning, Please send the invoice.", "Please send the invoice."},
		{"a comma with no space after it", "Hi Bob,please send the contract", "please send the contract"},
		{"a name on the line under the greeting", "Hi,\nJohn Smith,\nPlease send the contract.", "Please send the contract."},
		{"an ask after a mid-message thanks", "Thanks!\nPlease send the contract", "Thanks!\nPlease send the contract"},
		{"a postscript under a sign-off", "Regards,\nBob\nP.S. Please send the invoice", "Regards,\nBob\nP.S. Please send the invoice"},
		{"a phone number and a mail address", "Please send it.\nKind regards,\nJane Smith\nHead of Sales\nAcme Corp\n+49 30 12345\njane@acme.example", "Please send it."},
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
		{"a漢", 2, false},
	} {
		if got := WordBoundary(c.text, c.at); got != c.want {
			t.Errorf("WordBoundary(%q, %d) = %v, want %v", c.text, c.at, got, c.want)
		}
	}
}

// A quote that is only a salutation never counts, even where Strip kept it.
func TestASalutationIsNeverQuotedPast(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, text, quote string
		want              bool
	}{
		{"a greeting run on into the message", "Hi Marcus I wanted to ask about the price", "Hi Marcus", false},
		{"a closing above too many signature lines", "Kind regards\nOne Two\nThree Four\nFive Six\nSeven Eight\nNine Ten\nEleven", "Kind regards", false},
		{"a message that is only a thanks", "Thanks again", "Thanks again", false},
		{"the ask itself", "Hi Marcus I wanted to ask about the price", "ask about the price", true},
	} {
		if got := QuotedPast(c.text, c.quote); got != c.want {
			t.Errorf("%s: QuotedPast = %v, want %v", c.name, got, c.want)
		}
	}
}
