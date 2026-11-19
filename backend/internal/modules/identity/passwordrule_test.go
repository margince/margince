// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"encoding/json"
	"errors"
	"os"
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
	for _, pw := range []string{"someone@example.test.", "someone@EXAMPLE.test."} {
		if err := passwordEmailError("new_password", pw, email); err == nil {
			t.Errorf("password %q (trailing dot) was accepted for %s", pw, email)
		}
	}
	const unicode = "anna@bücher.example"
	for _, pw := range []string{"anna@xn--bcher-kva.example", "Anna@Bücher.example.", unicode} {
		if err := passwordEmailError("new_password", pw, unicode); err == nil {
			t.Errorf("password %q (punycode or Unicode spelling) was accepted for %s", pw, unicode)
		}
		if err := passwordEmailError("new_password", unicode, pw); err == nil {
			t.Errorf("account %q was not matched by the password %s", pw, unicode)
		}
	}
	if err := passwordEmailError("new_password", "anna@other.example", unicode); err != nil {
		t.Errorf("another domain was refused: %v", err)
	}
	if err := passwordEmailError("new_password", "another@example.test", email); err != nil {
		t.Errorf("a different address as password = %v, want nil", err)
	}
}

// The corpus is shared with frontend/src/features/sign-in/passwordrule.test.ts,
// so the two spellings of the rule are held to the same behaviour.
func TestPasswordRuleMatchesTheSharedCorpus(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/passwordrule.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus []struct{ Note, Password, Fault string }
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus) < 30 {
		t.Fatalf("the corpus holds %d cases, want its full census", len(corpus))
	}
	for _, c := range corpus {
		err := passwordRuleError("new_password", c.Password)
		got := "ok"
		if parseErr := new(values.ParseError); errors.As(err, &parseErr) {
			got = parseErr.Code
		}
		if got != c.Fault {
			t.Errorf("%s: refused as %q, the corpus says %q", c.Note, got, c.Fault)
		}
	}
}
