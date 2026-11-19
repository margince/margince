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
		{"a malformed email", VCardEntry{FullName: "A", Emails: []VCardChannel{{Value: "not-an-email"}}}, "email address"},
		{"a phone without a country prefix", VCardEntry{FullName: "A", Phones: []VCardChannel{{Value: "0301234567"}}}, "phone number"},
		{"an unreadable property", VCardEntry{FullName: "A", Unreadable: []string{"TITLE"}}, "job title"},
		{"an unreadable name", VCardEntry{Unreadable: []string{"FN"}}, "the name on this card is not readable"},
		{"no name at all", VCardEntry{Emails: []VCardChannel{{Value: "a@example.com"}}}, "the card states no name"},
	}
	for _, tc := range cases {
		if got := cardRefusal(tc.entry); !strings.Contains(got, tc.want) {
			t.Errorf("%s: refusal = %q, want it to contain %q", tc.name, got, tc.want)
		}
	}
	for _, secret := range []string{"not-an-email", "0301234567"} {
		entry := VCardEntry{FullName: "A", Emails: []VCardChannel{{Value: "not-an-email"}}, Phones: []VCardChannel{{Value: "0301234567"}}}
		if got := cardRefusal(entry); strings.Contains(got, secret) {
			t.Errorf("refusal %q repeats the value %q, which would reach the logs", got, secret)
		}
	}
	ok := VCardEntry{FullName: "A", Emails: []VCardChannel{{Value: "a@example.com"}}, Phones: []VCardChannel{{Value: "+4930123456"}}}
	if got := cardRefusal(ok); got != "" {
		t.Errorf("a valid card was refused: %q", got)
	}
}
