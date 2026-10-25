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
		name, from, to, address, want string
		found                         bool
	}{
		{"a sender with a display name", "DV <dv@example.com>", "owner@example.test", "dv@example.com", "DV", true},
		{"a bare sender address", "dv@example.com", "owner@example.test", "dv@example.com", "", true},
		{"a recipient of the owner's mail", "owner@example.test", "Dana V <DV@example.com>", "dv@example.com", "Dana V", true},
		{"an address the message does not name", "anna@example.com", "owner@example.test", "dv@example.com", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := strings.Join([]string{
				"From: " + tc.from, "To: " + tc.to, "Subject: hello", "Message-ID: <m1@example.com>", "", "body",
			}, "\r\n")
			got, found, err := CounterpartyNameOf([]byte(raw), tc.address)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want || found != tc.found {
				t.Errorf("CounterpartyNameOf(%q) = %q, %v; want %q, %v", tc.address, got, found, tc.want, tc.found)
			}
		})
	}
}
