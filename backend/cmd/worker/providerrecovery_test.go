// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "testing"

func TestParseReopenWindowRefusesWhatIsNotAWindow(t *testing.T) {
	cases := map[string][2]string{
		"missing end":   {"2026-10-01T00:00:00Z", ""},
		"not a time":    {"yesterday", "2026-10-02T00:00:00Z"},
		"ends first":    {"2026-10-02T00:00:00Z", "2026-10-01T00:00:00Z"},
		"empty between": {"2026-10-01T00:00:00Z", "2026-10-01T00:00:00Z"},
	}
	for name, c := range cases {
		if _, err := parseReopenWindow(c[0], c[1]); err == nil {
			t.Errorf("%s: accepted %q..%q", name, c[0], c[1])
		}
	}
	w, err := parseReopenWindow("2026-10-01T00:00:00+02:00", "2026-10-02T00:00:00Z")
	if err != nil || !w.From.Before(w.To) {
		t.Fatalf("a valid window was refused: %v", err)
	}
}
