// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "testing"

func TestLinkTypeValueNamesTheDeclaringUnit(t *testing.T) {
	if got := LinkTypeValue("finance-demo", "follow_up_of"); got != "ext:finance-demo:follow_up_of" {
		t.Errorf("LinkTypeValue = %q", got)
	}
}

func TestLinkTypeValidate(t *testing.T) {
	deal := []string{"deal"}
	cases := []struct {
		name string
		in   LinkType
		ok   bool
	}{
		{"plain", LinkType{Type: "follow_up_of", Label: "Follow-up of", From: deal, To: deal}, true},
		{"any end", LinkType{Type: "related", Label: "Related"}, true},
		{"empty type", LinkType{Label: "x"}, false},
		{"upper case", LinkType{Type: "Follow", Label: "x"}, false},
		{"leading digit", LinkType{Type: "1st", Label: "x"}, false},
		{"colon", LinkType{Type: "a:b", Label: "x"}, false},
		{"too long", LinkType{Type: "a23456789012345678901234567890123", Label: "x"}, false},
		{"no label", LinkType{Type: "related"}, false},
		{"unknown end", LinkType{Type: "related", Label: "x", From: []string{"project"}}, false},
		{"extension end", LinkType{Type: "related", Label: "x", To: []string{"ext:u:visit"}}, false},
	}
	for _, c := range cases {
		if err := c.in.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: Validate() = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
