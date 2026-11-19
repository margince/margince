// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"testing"
)

func TestIsWebAddress(t *testing.T) {
	t.Parallel()
	cases := []struct {
		value string
		want  bool
	}{
		{"https://example.com/offer?id=4", true},
		{"http://erp.internal:8080/orders/44", true},
		{"HTTP://EXAMPLE.COM/", true},
		{"HtTpS://example.com", true},
		{"javascript:alert(1)", false},
		{"JAVASCRIPT:alert(1)", false},
		{"data:text/html;base64,PHNjcmlwdD4=", false},
		{"example.com/offer", false},
		{"//example.com/offer", false},
		{"ftp://example.com/file", false},
		{"https://", false},
		{"https:///path", false},
		{"https://bank.example@evil.example/", false},
		{" https://example.com", false},
		{"https://example.com/\nnext", false},
		{"", false},
		{"https://example.com/" + strings.Repeat("a", WebAddressCharacters), false},
	}
	for _, c := range cases {
		if got := IsWebAddress(c.value); got != c.want {
			t.Errorf("IsWebAddress(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}
