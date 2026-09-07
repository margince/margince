// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A container rule names a provider and a place inside it, and the value is
// stored as the provider gave it.
//
// The two arms beside it fold their values, because an address and a domain are
// case-insensitive by their own specifications. A container id is not: a Graph
// folder id is base64url, where two distinct folders can differ only in case,
// and an IMAP mailbox name is case-sensitive except for INBOX. Folding one
// would merge two containers into a single rule and keep out mail the owner
// never asked to keep out — silently, because the rule would look right.

import "testing"

func TestAContainerRuleKeepsTheProvidersOwnSpelling(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, raw, want string
	}{
		{"a Gmail label id keeps its case", "gmail:Label_7", "gmail:Label_7"},
		{"a Graph folder id keeps its case", "graph:AAMkAGI2Tg", "graph:AAMkAGI2Tg"},
		{"an IMAP path keeps its separators and its case", "imap:INBOX/Family", "imap:INBOX/Family"},
		{"the PROVIDER folds, because that half is ours", "GMAIL:Label_7", "gmail:Label_7"},
		{"surrounding space is not part of either half", "  imap:INBOX  ", "imap:INBOX"},
		{"a container may itself contain a colon", "graph:AAMk:Tg", "graph:AAMk:Tg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidExclusionValue(ExclusionKindContainer, tc.raw)
			if err != nil {
				t.Fatalf("ValidExclusionValue(%q) = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("ValidExclusionValue(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// A rule that can never match is worse than a refusal: mail keeps arriving and
// nothing says why the rule the owner wrote does nothing.
func TestAContainerRuleThatCouldNeverMatchIsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, raw string }{
		{"no provider at all", "Private"},
		{"a provider nothing here speaks", "outlook:Private"},
		{"a provider with nothing after it", "gmail:"},
		{"a container with no provider before it", ":Label_7"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidExclusionValue(ExclusionKindContainer, tc.raw)
			var invalid *InvalidExclusionError
			if !asInvalid(err, &invalid) || invalid.Field != "value" {
				t.Fatalf("ValidExclusionValue(%q) = %q, %v — want a refusal naming the value", tc.raw, got, err)
			}
		})
	}
}

// asInvalid is errors.As pinned to this package's own refusal, so the two cases
// above read as one question rather than carrying the plumbing twice.
func asInvalid(err error, target **InvalidExclusionError) bool {
	invalid, ok := err.(*InvalidExclusionError) //nolint:errorlint // the refusal is returned directly, never wrapped
	if ok {
		*target = invalid
	}
	return ok
}
