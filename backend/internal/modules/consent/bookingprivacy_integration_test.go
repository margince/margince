// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import "testing"

func TestBookingPrivacyInformationUsesThePublishedPageThenTheContact(t *testing.T) {
	e := setupChannelConsent(t)
	for _, c := range []struct{ page, contact, want string }{
		{"https://example.test/privacy", "privacy@example.test", "https://example.test/privacy"},
		{"", "privacy@example.test", "privacy@example.test"},
		{"", "", ""},
	} {
		stateParticulars(t, e, ControllerParticulars{PrivacyPage: c.page, PrivacyContact: c.contact})
		got, err := e.store.BookingPrivacyInformation(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
