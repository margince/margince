// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"testing"
)

func TestCardRefusalNamesTheValueThatCannotBeStored(t *testing.T) {
	cases := []struct {
		name  string
		entry VCardEntry
		want  string
	}{
		{"a malformed email", VCardEntry{FullName: "A", Emails: []VCardChannel{{Value: "not-an-email"}}}, `"not-an-email"`},
		{"a phone without a country prefix", VCardEntry{FullName: "A", Phones: []VCardChannel{{Value: "0301234567"}}}, `"0301234567"`},
		{"an unreadable property", VCardEntry{FullName: "A", Unreadable: []string{"ORG"}}, "company"},
		{"an unreadable name", VCardEntry{Unreadable: []string{"FN"}}, "name"},
	}
	for _, tc := range cases {
		if got := cardRefusal(tc.entry); !strings.Contains(got, tc.want) {
			t.Errorf("%s: refusal = %q, want it to contain %q", tc.name, got, tc.want)
		}
	}
	ok := VCardEntry{FullName: "A", Emails: []VCardChannel{{Value: "a@example.com"}}, Phones: []VCardChannel{{Value: "+4930123456"}}}
	if got := cardRefusal(ok); got != "" {
		t.Errorf("a valid card was refused: %q", got)
	}
}
