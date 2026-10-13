// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package statedreason

import (
	"strings"
	"testing"
)

func TestAReasonIsAdmittedOnlyWhenStatedWithinTheBoundAsSent(t *testing.T) {
	for name, tc := range map[string]struct {
		reason string
		want   string
		ok     bool
	}{
		"a plain reason": {"wrong project", "wrong project", true},
		"a reason padded at the edges is trimmed":        {"  wrong project \n", "wrong project", true},
		"whitespace is not a reason":                     {" \t\n ", "", false},
		"empty":                                          {"", "", false},
		"exactly the bound":                              {strings.Repeat("é", Max), strings.Repeat("é", Max), true},
		"one past the bound":                             {strings.Repeat("x", Max+1), "", false},
		"within the bound once trimmed, but not as sent": {strings.Repeat(" ", 3) + strings.Repeat("x", Max), "", false},
	} {
		got, ok := Trim(tc.reason)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: Trim = (%q, %v), want (%q, %v)", name, got, ok, tc.want, tc.ok)
		}
	}
}
