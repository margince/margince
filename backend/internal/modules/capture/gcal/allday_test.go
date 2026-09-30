// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
)

// An all-day event is stated as a date. Its record says so, so the sink keys it
// by the date rather than by the noon it is stored at — the date is what an
// importer states for the same meeting.
func TestAnAllDayEventCarriesItsDateIdentity(t *testing.T) {
	raw := []byte(`{"id":"evt-ad","iCalUID":"uid-1","status":"confirmed","summary":"GERMANY",
		"start":{"date":"2026-07-19"},"end":{"date":"2026-08-12"},
		"organizer":{"email":"` + gcalOwner + `"},"attendees":[{"email":"client@acme.com"}]}`)
	ev, err := decodeEvent(raw, gcalOwner)
	if err != nil {
		t.Fatalf("decodeEvent: %v", err)
	}
	if !ev.AllDay {
		t.Fatal("an event stated as a date decoded as timed")
	}
	rec := meetingmap.Classify(ev, gcalOwner).ToRecord(connectorName, raw)
	id := rec.CrossDoorIdentity
	if !id.AllDay || id.Series != "uid-1" || !id.Occurrence.Equal(time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("identity = %+v, want uid-1 on 19 July, all-day", id)
	}
}

// A timed event is not all-day, even at noon UTC.
func TestATimedEventAtNoonIsNotAllDay(t *testing.T) {
	ev, err := decodeEvent(eventJSON(t, "evt-t", "confirmed", "Lunch", "2026-07-19T12:00:00Z", gcalOwner, "client@acme.com"), gcalOwner)
	if err != nil {
		t.Fatalf("decodeEvent: %v", err)
	}
	if ev.AllDay {
		t.Fatal("a timed event decoded as all-day")
	}
}
