// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Scheduling (features/07 §5c): availability proposes business-hour
// slots minus the host's existing meetings; booking commits one slot,
// lands a meeting on the linked timeline, and a taken slot answers
// slot_taken instead of double-booking.

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestAvailabilityAndBooking(t *testing.T) {
	e := setupRelationships(t)

	// A Tuesday-to-Wednesday window keeps the fixture off weekends.
	from := time.Date(2026, 7, 7, 8, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	window := fmt.Sprintf("from=%s&to=%s&duration_minutes=60",
		from.Format(time.RFC3339), to.Format(time.RFC3339))

	var avail struct {
		Slots []struct {
			Start time.Time `json:"start"`
			End   time.Time `json:"end"`
		} `json:"slots"`
	}
	if status := e.Call(t, "GET", "/v1/availability?"+window, nil, nil, &avail); status != http.StatusOK {
		t.Fatalf("availability → %d", status)
	}
	if len(avail.Slots) == 0 {
		t.Fatal("no slots in a free business day")
	}
	for _, s := range avail.Slots {
		if s.Start.Hour() < 9 || s.End.Hour() > 17 {
			t.Fatalf("slot outside business hours: %+v", s)
		}
	}
	first := avail.Slots[0]

	// Book it, linked to the contact.
	var booked struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	if status := e.Call(t, "POST", "/v1/bookings", AnyMap{
		"start": first.Start, "end": first.End, "subject": "Discovery call",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": e.contactID}},
	}, nil, &booked); status != http.StatusCreated || booked.Kind != "meeting" {
		t.Fatalf("book → %d %+v", status, booked)
	}

	// The same slot is now taken…
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "POST", "/v1/bookings", AnyMap{
		"start": first.Start, "end": first.End,
		"links": []AnyMap{{"entity_type": "contact", "entity_id": e.contactID}},
	}, nil, &problem); status != http.StatusConflict || problem.Code != "slot_taken" {
		t.Fatalf("double-book → %d %q, want 409 slot_taken", status, problem.Code)
	}
	// …and availability no longer proposes it.
	if status := e.Call(t, "GET", "/v1/availability?"+window, nil, nil, &avail); status != http.StatusOK {
		t.Fatalf("availability after booking → %d", status)
	}
	for _, s := range avail.Slots {
		if s.Start.Equal(first.Start) {
			t.Fatalf("booked slot still proposed: %+v", s)
		}
	}

	// A link target outside visibility refuses the booking (H1).
	if status := e.Call(t, "POST", "/v1/bookings", AnyMap{
		"start": first.Start.Add(3 * time.Hour), "end": first.End.Add(3 * time.Hour),
		"links": []AnyMap{{"entity_type": "contact", "entity_id": "00000000-0000-7000-8000-00000000dead"}},
	}, nil, nil); status != http.StatusNotFound {
		t.Fatalf("booking with invisible link → %d, want 404", status)
	}
}

// The overlap guard belongs to the booking door, and to nothing else.
//
// activity_meeting_no_overlap refuses a second meeting in a host's hour, and
// reads `claims_host_slot` to know whose rows it is holding. The door above
// answers slot_taken from its own availability read, so that test would pass
// even with the backstop disarmed — this is where the flag itself is asserted,
// because a constraint nothing arms is a guard that only looks like one.
//
// The other half is why the flag exists. A rep writing up their morning logs
// the 10:00 and the 10:30 they were actually in, and both now name them as
// host — that is how a meeting a colleague minutes stops counting into the
// minute-taker's week. Those rows overlap in the hour the constraint measures.
// Refusing the second would be refusing to record a Tuesday that has already
// happened: logging claims nothing on anyone's calendar, so the guard does not
// read those rows at all.
func TestOnlyABookingClaimsTheHostsHour(t *testing.T) {
	e := setupRelationships(t)
	at := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	link := []AnyMap{{"entity_type": "contact", "entity_id": e.contactID}}

	var booked struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/bookings", AnyMap{
		"start": at.Add(4 * time.Hour), "end": at.Add(5 * time.Hour),
		"subject": "Discovery call", "links": link,
	}, nil, &booked); status != http.StatusCreated {
		t.Fatalf("booking → %d", status)
	}
	if !claimsHostSlot(t, e, booked.ID) {
		t.Fatal("a booked meeting claims no slot, so the overlap guard holds nothing and a concurrent booking takes the same hour")
	}

	for _, minutes := range []int{0, 30} {
		occurred := at.Add(time.Duration(minutes) * time.Minute)
		var logged struct {
			ID         string `json:"id"`
			HostUserID string `json:"host_user_id"`
		}
		status := e.Call(t, "POST", "/v1/activities", AnyMap{
			"kind": "meeting", "subject": fmt.Sprintf("Call at %s", occurred.Format("15:04")),
			"occurred_at": occurred, "source": "manual", "links": link,
		}, nil, &logged)
		if status != http.StatusCreated {
			t.Fatalf("logging the meeting at %s → %d, want 201 — a rep's own back-to-back calls are not a double booking",
				occurred.Format("15:04"), status)
		}
		if logged.HostUserID == "" {
			t.Fatalf("the meeting at %s named no host, so it still counts for whoever typed it up",
				occurred.Format("15:04"))
		}
		if claimsHostSlot(t, e, logged.ID) {
			t.Fatalf("the meeting logged at %s claims its host's hour, which makes writing up a morning a double booking",
				occurred.Format("15:04"))
		}
	}
}

// claimsHostSlot reads the column the exclusion constraint reads. It is not on
// the wire and must not be: a guard a caller can opt out of is not one.
func claimsHostSlot(t *testing.T, e *relEnv, activityID string) bool {
	t.Helper()
	var claims bool
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT claims_host_slot FROM activity WHERE id = $1`, activityID).Scan(&claims); err != nil {
		t.Fatalf("reading the slot claim of %s: %v", activityID, err)
	}
	return claims
}
