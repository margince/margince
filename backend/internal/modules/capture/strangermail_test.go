// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The judgements behind bcc_only and foreign_link, over the envelope the mail
// parser hands the sink.

import (
	"context"
	"slices"
	"testing"
)

func TestTheMailboxIsAddressedOpenlyOnlyWhenToOrCcNamesTheSeat(t *testing.T) {
	self := NewSelfSetWithIdentityOnly([]string{"reader@mailhost.example", "alias@mailhost.example"},
		[]string{"login@mailhost.example"}, []string{"mailhost.example"})
	cases := map[string]struct {
		recipients  []string
		deliveredTo string
		want        bool
	}{
		"Bcc only, the sender in To":      {recipients: []string{"sender@example.com"}, want: false},
		"undisclosed recipients":          {recipients: nil, want: false},
		"a colleague on our domain in Cc": {recipients: []string{"colleague@ourco.example"}, want: false},
		"a colleague under the seat's domain claim": {
			recipients: []string{"colleague@mailhost.example"}, want: false,
		},
		"the seat's login address":     {recipients: []string{"login@mailhost.example"}, want: true},
		"the reader in Cc":             {recipients: []string{"other@example.com", "reader@mailhost.example"}, want: true},
		"an alias of the reader in To": {recipients: []string{"Alias@MailHost.example"}, want: true},
		"the address it was delivered to": {
			recipients: []string{"inbox@elsewhere.example"}, deliveredTo: "inbox@elsewhere.example", want: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := addressedOpenly(tc.recipients, tc.deliveredTo, self); got != tc.want {
				t.Errorf("addressedOpenly(%v) = %v, want %v", tc.recipients, got, tc.want)
			}
		})
	}
}

// A link list the parser cut short is decided as foreign before any read: the
// unread rest could hold a foreign link.
func TestALinkListCutShortCountsAsForeign(t *testing.T) {
	foreign, err := foreignLinkTx(context.Background(), nil, "sender@example.com",
		&MailEnvelope{LinkHosts: []string{"example.com"}, LinksTruncated: true})
	if err != nil || !foreign {
		t.Errorf("a cut-short link list read foreign=%v err=%v, want foreign", foreign, err)
	}
}

func TestALinkIsUnrelatedOnlyWhenItLeavesTheSendersSite(t *testing.T) {
	own := NewInternalDomains([]string{"ourco.example"})
	cases := map[string]struct {
		sender string
		hosts  []string
		want   []string
	}{
		"a redirect's real target on another site": {
			sender: "sender@example.com", hosts: []string{"landing.example.org"}, want: []string{"example.org"},
		},
		"the sender's own domain":            {sender: "sender@example.com", hosts: []string{"example.com"}},
		"a subdomain of the sender's domain": {sender: "sender@example.com", hosts: []string{"docs.example.com"}},
		"our own site":                       {sender: "sender@example.com", hosts: []string{"www.ourco.example"}},
		"one foreign link among related ones": {
			sender: "sender@example.com", hosts: []string{"example.com", "landing.example.org"}, want: []string{"example.org"},
		},
		"a sibling under a public suffix": {sender: "a@shop.example.co.uk", hosts: []string{"www.example.co.uk"}},
		"another site under a public suffix": {
			sender: "a@shop.co.uk", hosts: []string{"other.co.uk"}, want: []string{"other.co.uk"},
		},
		"an IP-literal sender linking its own address": {sender: "user@[192.0.2.1]", hosts: []string{"192.0.2.1"}},
		"an IP-literal sender linking another address": {
			sender: "user@[192.0.2.1]", hosts: []string{"198.51.100.7"}, want: []string{"198.51.100.7"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := unrelatedSites(tc.sender, tc.hosts, own); !slices.Equal(got, tc.want) {
				t.Errorf("unrelatedSites(%s, %v) = %v, want %v", tc.sender, tc.hosts, got, tc.want)
			}
		})
	}
}

// Mail from one sender may store its address with the domain in IDNA form or in
// unicode. The repair after a reply reaches both.
func TestStoredSpellingsCoverBothFormsOfAUnicodeDomain(t *testing.T) {
	cases := map[string][]string{
		"Sam@Example.com":           {"sam@example.com"},
		"sam@xn--bcher-kva.example": {"sam@xn--bcher-kva.example", "sam@bücher.example"},
		"sam@Bücher.example":        {"sam@bücher.example", "sam@xn--bcher-kva.example"},
	}
	for address, want := range cases {
		if got := storedSpellings(address); !slices.Equal(got, want) {
			t.Errorf("storedSpellings(%s) = %v, want %v", address, got, want)
		}
	}
}
