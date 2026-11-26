// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "testing"

// Marketing mode follows the recipient's pack, never the sender's. A German
// entity may write to a US recipient. A US entity needs consent for a German one.
func TestTheMarketingModeFollowsTheRecipientsPackNotTheSenders(t *testing.T) {
	t.Parallel()
	codes := shippedPackCodes(t)
	for _, tc := range []struct {
		name              string
		recipient, sender string
		want              bool
	}{
		{"a German sender to a US recipient", "us", "de", true},
		{"a US sender to a German recipient", "de", "us", false},
		{"a US sender to a US recipient", "us", "us", true},
		{"a German sender to a German recipient", "de", "de", false},
		{"a US sender to a recipient with no country", "", "us", false},
	} {
		country := RecipientCountry{Code: codes[tc.recipient], Sender: codes[tc.sender]}
		if got := packRulesFor(country).optOut; got != tc.want {
			t.Errorf("%s: opt-out = %v, want %v", tc.name, got, tc.want)
		}
	}
}
