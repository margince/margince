// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// The calendar set is the OAuth set minus the mail and meeting-notes sets.
// It has to come out as the two calendars this product connects.
// Derivation can empty it: a mail list covering every OAuth provider would
// tell every host their calendar is unconnected.
func TestTheCalendarProvidersAreTheOAuthProvidersThatAreNotMailOrNotes(t *testing.T) {
	want := []string{providerGcal, providerGraphCal}
	if !slices.Equal(calendarProviders, want) {
		t.Fatalf("calendarProviders = %v, want %v", calendarProviders, want)
	}
	for _, provider := range calendarProviders {
		if IsMailProvider(provider) {
			t.Errorf("%q connects a mailbox and is listed as a calendar", provider)
		}
		if slices.Contains(capture.MeetingNotesProviders(), provider) {
			t.Errorf("%q reads meeting notes and is listed as a calendar", provider)
		}
	}
}
