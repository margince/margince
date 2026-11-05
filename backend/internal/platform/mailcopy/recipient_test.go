// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestMoneyFormattingPreservesExactCurrencyUnits(t *testing.T) {
	for _, tc := range []struct {
		language, currency string
		minor              int64
		want               string
	}{
		{"de", "EUR", 4200000, "42.000,00 EUR"},
		{"en", "EUR", 4200000, "42,000.00 EUR"},
		{"vi", "VND", 1284000, "1.284.000 VND"},
		{"de", "JPY", -1234, "-1.234 JPY"},
		{"en", "JPY", math.MinInt64, "-9,223,372,036,854,775,808 JPY"},
	} {
		if got := For(tc.language).Money(tc.minor, tc.currency); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.language, got, tc.want)
		}
	}
}

func TestDeadlineRetainsTheActualLocalTimeAndOffset(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Date(2026, 11, 15, 12, 37, 0, 0, zone)
	got := For("de").Instant(deadline)
	for _, want := range []string{"15.11.2026", "12:37", "UTC+01:00"} {
		if !strings.Contains(got, want) {
			t.Errorf("deadline %q lost %q", got, want)
		}
	}
}

func TestExternalGermanUsesFormalAddressAndExplainsAccess(t *testing.T) {
	words := For("de")
	message := words.RoomMessage("Angebot", "Dana", "Example Company", "https://example.test/#/room?c=fictional", time.Date(2026, 11, 15, 12, 37, 0, 0, time.UTC))
	body := message.Text
	for _, want := range []string{"Dana von Example Company", "für Sie", "12:37", "Anmeldungen und Downloads", "neuen Link anfordern"} {
		if !strings.Contains(body, want) {
			t.Errorf("room invitation lost %q", want)
		}
	}
	if strings.Contains(body, "dein") {
		t.Fatal("external invitation switched to informal address")
	}
}

func TestMeetingReminderKeepsTheIntervalAcrossMidnight(t *testing.T) {
	start := time.Date(2026, 11, 15, 23, 45, 0, 0, time.UTC)
	message := For("de").MeetingMessage("Angebot besprechen", "Zweijahresvertrag", "Videoanruf", "https://example.test/manage", start, start.Add(45*time.Minute))
	body := message.Text
	for _, want := range []string{"Ihr Termin", "15.11.2026, 23:45", "16.11.2026, 00:30", "45 Min.", "Termin verschieben oder absagen"} {
		if !strings.Contains(body, want) {
			t.Errorf("reminder lost %q: %s", want, body)
		}
	}
}
