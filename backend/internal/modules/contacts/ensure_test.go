// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The fourth mail-domain gate of the capture ladder, and the one that is easy
// to miss because it lives here rather than in capture: both tells it reads are
// statements ABOUT the sender's mail domain, so a record that has none must
// pass it as a no-op. Without the floor the embedded-address tell compares
// against the empty string and flags every display name containing an "@" —
// suppressing a legitimate conversation for a reason that cannot apply to it.
func TestAnImpersonationSignNeedsADomainToJudge(t *testing.T) {
	cases := []struct {
		name        string
		displayName string
		domain      string
		want        string
	}{
		{"a punycode domain is a homoglyph vector", "Acme Billing", "xn--80ak6aa92e.com", impersonationPunycodeDomain},
		{"a punycode label below the apex counts too", "Acme Billing", "mail.xn--80ak6aa92e.com", impersonationPunycodeDomain},
		{"an embedded address on a foreign domain is the spoof sign", "ceo@real-corp.example", "spoof.test", impersonationDisplayNameOtherDomain},
		{"an embedded address on the sending domain is just a signature", "ceo@acme.test", "acme.test", ""},
		{"an ordinary name on an ordinary domain", "Carol Example", "acme.test", ""},
		{"a channel sender's name that happens to contain an address", "ceo@real-corp.example", "", ""},
		{"a channel sender with a plain name", "Anna Schmidt", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := impersonationSign(tc.displayName, tc.domain); got != tc.want {
				t.Fatalf("impersonationSign(%q, %q) = %q, want %q", tc.displayName, tc.domain, got, tc.want)
			}
		})
	}
}

// The name a channel record stores when the provider gave a poor one. contact
// pins full_name NOT NULL and there is no address whose local part could stand
// in, so the ladder ends at the identity key itself — never at an empty string.
func TestChannelCounterpartyNameFallsBackToTheHandleThenTheKey(t *testing.T) {
	ci := connector.ChannelIdentity{Provider: "telegram", ChannelUserID: "880001", Username: "annahandle"}
	if got := channelCounterpartyName("  Anna Schmidt  ", ci); got != "Anna Schmidt" {
		t.Fatalf("name = %q, want the trimmed provider name", got)
	}
	if got := channelCounterpartyName("   ", ci); got != "@annahandle" {
		t.Fatalf("name = %q, want the handle", got)
	}
	handleless := ci
	handleless.Username = ""
	if got := channelCounterpartyName("", handleless); got != "telegram:880001" {
		t.Fatalf("name = %q, want the identity key", got)
	}
}
