// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestASignatureQuotesAnAddressOnlyAsAWholeToken(t *testing.T) {
	for snippet, want := range map[string]bool{
		"Dana | victim@acme.example | +49 30 1":   true,
		"Dana | VICTIM@acme.example":              true,
		"Dana | notvictim@acme.example":           false,
		"Dana | victim@acme.example.evil.example": false,
		"Dana | no address here":                  false,
	} {
		if got := quotesAddress(snippet, "victim@acme.example"); got != want {
			t.Errorf("quotesAddress(%q) = %v, want %v", snippet, got, want)
		}
	}
}

func TestASignatureQuotesAValueOnlyWhenItsSnippetPrintsIt(t *testing.T) {
	for value, want := range map[string]bool{
		"Hafenstrasse 12, 20457 Hamburg":   true,
		"hafenstrasse  12,\n20457 hamburg": true,
		"Evil Road 1, 10115 Berlin":        false,
		"":                                 false,
	} {
		if got := quotesValue("Dana | Hafenstrasse 12, 20457 Hamburg | Germany", value); got != want {
			t.Errorf("quotesValue(%q) = %v, want %v", value, got, want)
		}
	}
}
