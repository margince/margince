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
		if code, _ := CompanyNameFault(name); code != want {
			t.Errorf("CompanyNameFault(%d runes) = %q, want %q", len([]rune(name)), code, want)
		}
	}
}
