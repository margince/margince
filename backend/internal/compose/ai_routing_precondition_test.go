// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"testing"
)

func TestRoutingPreconditionDistinguishesAbsentWildcardAndEmpty(t *testing.T) {
	for _, test := range []struct {
		name, value, expected string
		present, invalid      bool
	}{
		{name: "absent"},
		{name: "wildcard", value: "*", present: true},
		{name: "empty", present: true, invalid: true},
		{name: "empty quoted tag", value: `""`, present: true, invalid: true},
		{name: "revision", value: `"revision"`, expected: "revision", present: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			if test.present {
				header.Set("If-Match", test.value)
			}
			got, err := routingPrecondition(header)
			if got != test.expected || (err != nil) != test.invalid {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}
