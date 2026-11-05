// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

func TestRecipientCertificationRejectsMissingAndInventedRecipients(t *testing.T) {
	keys := map[string]string{"contact:a": "ines", "contact:b": "morgan"}
	for _, example := range []struct {
		name string
		refs []string
		want bool
	}{
		{"both", []string{"contact:b", "contact:a"}, true},
		{"missing", []string{"contact:a"}, false},
		{"duplicate", []string{"contact:a", "contact:a"}, false},
		{"invented", []string{"contact:a", "contact:c"}, false},
	} {
		t.Run(example.name, func(t *testing.T) {
			if got := expectedRecipientsMatch([]string{"ines", "morgan"}, example.refs, keys); got != example.want {
				t.Fatalf("match=%v, want %v", got, example.want)
			}
		})
	}
	if err := validateExpectedRecipients([]string{"mira"}, map[string]string{"mira": "seat:a"}); err == nil {
		t.Fatal("a colleague cannot be a contact recipient")
	}
}
