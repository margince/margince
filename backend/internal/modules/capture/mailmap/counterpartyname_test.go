// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strings"
	"testing"
)

func TestCounterpartyNameOfReadsTheNameTheHeaderGaveTheAddress(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, from, to, want string
		outbound, found      bool
	}{
		{"a sender with a display name", "DV <dv@example.com>", "owner@example.test", "DV", false, true},
		{"a quoted display name", `"DV" <dv@example.com>`, "owner@example.test", "DV", false, true},
		{"an encoded display name", "=?UTF-8?Q?DV?= <dv@example.com>", "owner@example.test", "DV", false, true},
		{"a display name with a trailing dot", `"DV." <dv@example.com>`, "owner@example.test", "DV.", false, true},
		{"a bare sender address", "dv@example.com", "owner@example.test", "", false, true},
		{"a recipient of the owner's mail", "owner@example.test", "Dana V <DV@example.com>", "Dana V", true, true},
		{"inbound mail read from From only", "anna@example.com", "DV <dv@example.com>", "", false, false},
		{"outbound mail read from To only", "DV <dv@example.com>", "owner@example.test", "", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := strings.Join([]string{
				"From: " + tc.from, "To: " + tc.to, "Subject: hello", "Message-ID: <m1@example.com>", "", "body",
			}, "\r\n")
			got, found, err := CounterpartyNameOf([]byte(raw), "dv@example.com", tc.outbound)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want || found != tc.found {
				t.Errorf("CounterpartyNameOf(outbound=%v) = %q, %v; want %q, %v", tc.outbound, got, found, tc.want, tc.found)
			}
		})
	}
}
