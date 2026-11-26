// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"testing"
)

func TestCompanyNameFaultNamesTheCodeAClientMaps(t *testing.T) {
	cases := map[string]string{
		"Acme GmbH":                     "",
		"   ":                           "required",
		r(0x200B):                       "required",
		strings.Repeat("a", 200):        "",
		strings.Repeat("a", 201):        "too_long",
		"  " + strings.Repeat("a", 200): "",
		strings.Repeat("組", 200):        "",
	}
	for name, want := range cases {
		code := ""
		if refused := CompanyNameFault("display_name", name); refused != nil {
			code = refused.Code
		}
		if code != want {
			t.Errorf("CompanyNameFault(%d runes) = %q, want %q", len([]rune(name)), code, want)
		}
	}
}

func TestCompanyNameFaultStatesTheLimitItRefusedAt(t *testing.T) {
	refused := CompanyNameFault("display_name", strings.Repeat("a", MaxCompanyNameRunes+1))
	if refused == nil || refused.FieldMax() != MaxCompanyNameRunes || refused.Field != "display_name" {
		t.Errorf("CompanyNameFault = %+v, want display_name refused at %d", refused, MaxCompanyNameRunes)
	}
}
