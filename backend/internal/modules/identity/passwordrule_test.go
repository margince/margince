// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestPasswordRuleErrorNamesWhyAPasswordIsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, password, code string
	}{
		{"too short", "short", "length"},
		{"four emoji are four characters", "🔑🔑🔑🔑", "length"},
		{"too long", strings.Repeat("ab", 129), "length"},
		{"twelve spaces", strings.Repeat(" ", 12), "blank"},
		{"tabs and newlines", " \t\n \t\n \t\n \t\n", "blank"},
		{"one letter repeated", strings.Repeat("a", 12), "repeated"},
		{"one emoji repeated", strings.Repeat("🔑", 12), "repeated"},
		{"accepted: 12 mixed characters", "abababababab", ""},
		{"accepted: spaces inside a phrase", "correct horse battery", ""},
		{"accepted: exactly 256", strings.Repeat("ab", 128), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := passwordRuleError("new_password", tc.password)
			if tc.code == "" {
				if err != nil {
					t.Fatalf("passwordRuleError = %v, want nil", err)
				}
				return
			}
			var parseErr *values.ParseError
			if !errors.As(err, &parseErr) || parseErr.Field != "new_password" || parseErr.Code != tc.code {
				t.Fatalf("passwordRuleError = %v, want a new_password/%s refusal", err, tc.code)
			}
		})
	}
}

func TestPasswordEmailErrorRefusesTheAccountAddressOnly(t *testing.T) {
	t.Parallel()
	const email = "someone@example.test"
	var parseErr *values.ParseError
	for _, pw := range []string{email, "Someone@Example.test", " " + email + " "} {
		if err := passwordEmailError("new_password", pw, email); !errors.As(err, &parseErr) || parseErr.Code != "email" {
			t.Errorf("password %q = %v, want a new_password/email refusal", pw, err)
		}
	}
	if err := passwordEmailError("new_password", "another@example.test", email); err != nil {
		t.Errorf("a different address as password = %v, want nil", err)
	}
}
