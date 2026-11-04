// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "testing"

func TestCompanyTypeValueNamesTheDeclaringUnit(t *testing.T) {
	if got := CompanyTypeValue("finance-demo", "portfolio_holding"); got != "ext:finance-demo:portfolio_holding" {
		t.Errorf("CompanyTypeValue = %q", got)
	}
}

func TestCompanyTypeValidate(t *testing.T) {
	cases := []struct {
		name string
		in   CompanyType
		ok   bool
	}{
		{"plain", CompanyType{Type: "reseller", Label: "Reseller"}, true},
		{"snake", CompanyType{Type: "portfolio_holding", Label: "Holding"}, true},
		{"digits", CompanyType{Type: "tier2", Label: "Tier 2"}, true},
		{"empty type", CompanyType{Label: "x"}, false},
		{"upper case", CompanyType{Type: "Reseller", Label: "x"}, false},
		{"hyphen", CompanyType{Type: "re-seller", Label: "x"}, false},
		{"colon", CompanyType{Type: "a:b", Label: "x"}, false},
		{"doubled underscore", CompanyType{Type: "a__b", Label: "x"}, false},
		{"leading underscore", CompanyType{Type: "_a", Label: "x"}, false},
		{"no label", CompanyType{Type: "reseller"}, false},
		{"blank label", CompanyType{Type: "reseller", Label: "  "}, false},
	}
	for _, c := range cases {
		err := c.in.Validate()
		if (err == nil) != c.ok {
			t.Errorf("%s: Validate() = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
