// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owndomain

import "testing"

// captureCallsInternal is how capture judges a meeting it read. Every attendee
// is on a confirmed domain (capture.internalOnlyTx), or every attendee is on
// the seat's domain and that domain is ours. An address with no readable
// domain is external, as meetingmap.classifyAttendees counts it.
func captureCallsInternal(p Parties, seat string, attendees []string) bool {
	trusted, onSeat := true, true
	for _, a := range attendees {
		trusted = trusted && p.Trusted.Covers(a)
		onSeat = onSeat && OfAddress(a) != "" && OfAddress(a) == OfAddress(seat)
	}
	return trusted || (onSeat && p.SeatDomainIsOurs(OfAddress(seat)))
}

func outsideAttendees(p Parties, seat string, attendees []string) int {
	n := 0
	for _, a := range attendees {
		if p.IsOutsideAttendee(a, []string{OfAddress(seat)}) {
			n++
		}
	}
	return n
}

var (
	consumerMail  = func(d string) bool { return d == "gmail.example" }
	noneConfirmed = Parties{Consumer: consumerMail}
	oneConfirmed  = Parties{Trusted: New([]string{"ourco.example"}), Possible: New([]string{"ourco.example", "labs.example"}), Consumer: consumerMail}
)

// Capture calls a meeting internal when deal evidence finds no outside attendee
// on it, and only then. The cases cover internal, external, consumer and
// candidate domains.
func TestCaptureAndDealEvidenceAgreeAboutAMeeting(t *testing.T) {
	meetings := []struct {
		seat      string
		attendees []string
	}{
		{"rep@ourco.example", []string{"colleague@ourco.example"}},
		{"rep@ourco.example", []string{"ines@customer.example"}},
		{"rep@ourco.example", []string{"colleague@ourco.example", "ines@customer.example"}},
		{"rep@gmail.example", []string{"friend@gmail.example"}},
		{"rep@labs.example", []string{"dev@labs.example"}},
		{"rep@ourco.example", []string{"dev@labs.example"}},
		{"rep@labs.example", []string{"colleague@ourco.example"}},
		{"rep@sub.ourco.example", []string{"colleague@ourco.example"}},
	}
	for _, parties := range []Parties{noneConfirmed, oneConfirmed} {
		for _, m := range meetings {
			outside := outsideAttendees(parties, m.seat, m.attendees)
			if internal := captureCallsInternal(parties, m.seat, m.attendees); internal != (outside == 0) {
				t.Errorf("trusted %v, seat %s, attendees %v: capture calls it internal = %v, deal evidence counts %d outside",
					parties.Trusted.Domains(), m.seat, m.attendees, internal, outside)
			}
		}
	}
}

// Two meetings capture keeps that prove no buyer attended. Keeping one loses
// nothing, while counting a buyer here would settle a criterion on a guess:
//   - an attendee whose address has no domain, so nobody knows who it is;
//   - colleagues split across the seat's candidate domain and a confirmed one.
func TestTheMeetingsCaptureKeepsWithoutAProvenBuyer(t *testing.T) {
	for _, m := range []struct {
		parties   Parties
		seat      string
		attendees []string
	}{
		{noneConfirmed, "rep@ourco.example", []string{"guest"}},
		{oneConfirmed, "rep@ourco.example", []string{"colleague@ourco.example", "guest"}},
		{oneConfirmed, "rep@labs.example", []string{"dev@labs.example", "colleague@ourco.example"}},
	} {
		if captureCallsInternal(m.parties, m.seat, m.attendees) {
			t.Errorf("seat %s, attendees %v: capture would drop the meeting as internal", m.seat, m.attendees)
		}
		if n := outsideAttendees(m.parties, m.seat, m.attendees); n != 0 {
			t.Errorf("seat %s, attendees %v: deal evidence counts %d outside attendees, want none", m.seat, m.attendees, n)
		}
	}
}
