// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The judgements behind bcc_only and foreign_link, over the envelope the mail
// parser hands the sink.

import (
	"slices"
	"testing"
)

func TestTheMailboxIsAddressedOpenlyOnlyWhenToOrCcNamesUs(t *testing.T) {
	self := NewSelfSet([]string{"reader@mailhost.example", "alias@mailhost.example"}, nil)
	own := NewInternalDomains([]string{"ourco.example"})
	cases := map[string]struct {
		recipients  []string
		deliveredTo string
		want        bool
	}{
		"Bcc only, the sender in To":       {recipients: []string{"sender@example.com"}, want: false},
		"undisclosed recipients":           {recipients: nil, want: false},
		"the reader in Cc":                 {recipients: []string{"other@example.com", "reader@mailhost.example"}, want: true},
		"an alias of the reader in To":     {recipients: []string{"Alias@MailHost.example"}, want: true},
		"the address it was delivered to":  {recipients: []string{"inbox@elsewhere.example"}, deliveredTo: "inbox@elsewhere.example", want: true},
		"a team address on our own domain": {recipients: []string{"sales@ourco.example"}, want: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := addressedOpenly(tc.recipients, tc.deliveredTo, self, own); got != tc.want {
				t.Errorf("addressedOpenly(%v) = %v, want %v", tc.recipients, got, tc.want)
			}
		})
	}
}

func TestALinkIsUnrelatedOnlyWhenItLeavesTheSendersSite(t *testing.T) {
	own := NewInternalDomains([]string{"ourco.example"})
	cases := map[string]struct {
		hosts []string
		want  []string
	}{
		"a redirect's real target on another site": {hosts: []string{"landing.example.org"}, want: []string{"example.org"}},
		"the sender's own domain":                  {hosts: []string{"example.com"}, want: nil},
		"a subdomain of the sender's domain":       {hosts: []string{"docs.example.com"}, want: nil},
		"our own site":                             {hosts: []string{"www.ourco.example"}, want: nil},
		"one foreign link among related ones":      {hosts: []string{"example.com", "landing.example.org"}, want: []string{"example.org"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := unrelatedSites("sender@example.com", tc.hosts, own); !slices.Equal(got, tc.want) {
				t.Errorf("unrelatedSites(%v) = %v, want %v", tc.hosts, got, tc.want)
			}
		})
	}
}
