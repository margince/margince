// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package formulacell

import "testing"

func TestGuardQuotesEveryFormulaLeadAndUnguardRestoresIt(t *testing.T) {
	cases := []struct{ raw, guarded string }{
		{"=HYPERLINK(\"x\")", "'=HYPERLINK(\"x\")"},
		{"+49 30 1234567", "'+49 30 1234567"},
		{"+12345", "'+12345"},
		{"+1234567890123456", "'+1234567890123456"},
		{"-5000", "'-5000"},
		{"- first point", "'- first point"},
		{"@mention", "'@mention"},
		{"\ttab", "'\ttab"},
		{"\rcr", "'\rcr"},
		{"'=x", "''=x"},
		{"''=x", "'''=x"},
		{"+84901234567", "+84901234567"},
		{"+123456", "+123456"},
		{"'til then", "'til then"},
		{"'", "'"},
		{"Acme", "Acme"},
		{"", ""},
	}
	for _, c := range cases {
		if got := Guard(c.raw); got != c.guarded {
			t.Errorf("Guard(%q) = %q, want %q", c.raw, got, c.guarded)
		}
		if got := Unguard(c.guarded); got != c.raw {
			t.Errorf("Unguard(%q) = %q, want %q", c.guarded, got, c.raw)
		}
	}
}

func TestUnguardCleansAPhoneAnOlderExportQuoted(t *testing.T) {
	if got := Unguard("'+84901234567"); got != "+84901234567" {
		t.Errorf("Unguard = %q, want the bare number", got)
	}
}

func FuzzGuardRoundTrip(f *testing.F) {
	for _, seed := range []string{"", "=1+1", "'=x", "''", "'til", "+84901234567", "'+84901234567", "+1 2", "-", "@", "\t", "\r\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got := Unguard(Guard(s)); got != s {
			t.Errorf("Unguard(Guard(%q)) = %q", s, got)
		}
	})
}
