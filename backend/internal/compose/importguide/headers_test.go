// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importguide

import (
	"strings"
	"testing"
)

func TestEveryHeaderAliasNamesAGuidedColumn(t *testing.T) {
	for key := range headerAliases {
		if _, ok := Columns[key]; !ok {
			t.Errorf("header aliases for %q, which no import column is keyed by", key)
		}
	}
}

func TestAHeaderNamesItsFieldByTheLabelInEveryLanguage(t *testing.T) {
	names := HeaderNames("company", []string{"display_name", "phone", "size_band"}, strings.ToLower)
	for header, want := range map[string]string{
		"company name": "display_name", "firmenname": "display_name", "tên công ty": "display_name",
		"telefon": "phone", "phone number": "phone", "mitarbeitende": "size_band", "name": "display_name",
	} {
		if names[header] != want {
			t.Errorf("%q → %q, want %q", header, names[header], want)
		}
	}
}

func TestANameTwoFieldsShareNamesNeither(t *testing.T) {
	names := HeaderNames("contact", []string{"full_name", "company_name"}, strings.ToLower)
	if names["company name"] != "company_name" {
		t.Fatalf("company name → %q, want company_name", names["company name"])
	}
	shared := HeaderNames("lead", []string{"description", "deal.description"}, strings.ToLower)
	if target, seen := shared["description"]; !seen || target != "" {
		t.Fatalf("a label two targets carry → %q (seen %v), want it held by neither", target, seen)
	}
}
