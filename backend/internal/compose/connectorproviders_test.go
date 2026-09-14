// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
)

// The calendar set is the OAuth set minus the mail set, and it has to come out
// as the two calendars this product actually connects.
//
// Derivation is what keeps it in step with the vocabulary, and it is also what
// can quietly empty it: a mail list that grew to cover every OAuth provider
// would leave no calendars, and the scheduling seam would then tell every host
// their calendar is unconnected — a true statement about nothing, raised so
// often that the reader it was written for stops reading it.
func TestTheCalendarProvidersAreTheOAuthProvidersThatAreNotMail(t *testing.T) {
	want := []string{providerGcal, providerGraphCal}
	if !slices.Equal(calendarProviders, want) {
		t.Fatalf("calendarProviders = %v, want %v", calendarProviders, want)
	}
	for _, provider := range calendarProviders {
		if IsMailProvider(provider) {
			t.Errorf("%q connects a mailbox and is listed as a calendar", provider)
		}
	}
}
