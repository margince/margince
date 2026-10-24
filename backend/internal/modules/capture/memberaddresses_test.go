// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

func TestAMessageIsMembersOnlyWhenEveryPartyIsAMember(t *testing.T) {
	members := MemberAddresses{known: NewSelfSet(
		[]string{"a@work.example", "b.private@home.example"},
		[]string{"work.example"},
	)}
	cases := []struct {
		name    string
		parties []string
		want    bool
	}{
		{"two members on their exact addresses", []string{"a@work.example", "B.Private@home.example"}, true},
		{"a member at a subdomain of an own domain", []string{"a@work.example", "sales@eu.work.example"}, true},
		{"one outside party", []string{"a@work.example", "b.private@home.example", "ext@home.example"}, false},
		{"an outside party on a sibling of a member's address", []string{"b.private@home.example", "b@home.example"}, false},
		{"no parties at all", nil, false},
		{"only blank parties", []string{"", "  "}, false},
	}
	for _, c := range cases {
		if got := members.CoverEveryParty(c.parties); got != c.want {
			t.Errorf("%s: CoverEveryParty(%v) = %v, want %v", c.name, c.parties, got, c.want)
		}
	}
}
