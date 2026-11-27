// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"testing"
)

func TestAnEmailReferenceKeepsLegacyLengthsAndAddressValidation(t *testing.T) {
	legacy := strings.Repeat("a", MaxEmailLength) + "@legacy.test"
	for _, tc := range []struct {
		input, want, code string
	}{
		{"  " + strings.ToUpper(legacy) + "  ", legacy, ""},
		{"Ada@Example.test", "ada@example.test", ""},
		{"  Ada@localhost  ", "ada@localhost", ""},
		{"", "", "email_empty"},
		{"not-an-email", "", "email_malformed"},
		{"Ada <ada@example.test>", "", "email_malformed"},
		{ErasedEmail, "", "email_reserved"},
	} {
		got, err := ParseEmailReference(tc.input)
		if tc.code != "" {
			wantParseError(t, err, tc.code)
			continue
		}
		if err != nil || got.String() != tc.want {
			t.Errorf("reference = %q, %v; want %q", got.String(), err, tc.want)
		}
	}
	_, err := ParseEmail(legacy)
	wantParseError(t, err, "too_long")
}
