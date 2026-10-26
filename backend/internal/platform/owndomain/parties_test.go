// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owndomain

import "testing"

// captureCallsInternal is how capture judges a meeting whose parties it read:
// every attendee on a confirmed domain, or every attendee on the seat's domain
// when that domain is ours (capture.internalOnlyTx).
func captureCallsInternal(p Parties, seat string, attendees []string) bool {
	trusted, onSeat := true, true
	for _, a := range attendees {
		trusted = trusted && p.Trusted.Covers(a)
		onSeat = onSeat && OfAddress(a) == OfAddress(seat)
	}
	return trusted || (onSeat && p.SeatDomainIsOurs(OfAddress(seat)))
}

// Capture calls a meeting internal exactly when deal evidence finds no outside
// attendee on it, for internal, external, consumer and candidate domains.
func TestCaptureAndDealEvidenceAgreeAboutAMeeting(t *testing.T) {
	consumer := func(d string) bool { return d == "gmail.example" }
	none := Parties{Consumer: consumer}
	confirmed := Parties{Trusted: New([]string{"ourco.example"}), Possible: New([]string{"ourco.example", "labs.example"}), Consumer: consumer}
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
	for _, parties := range []Parties{none, confirmed} {
		for _, m := range meetings {
			outside := 0
			for _, a := range m.attendees {
				if parties.IsOutsideAttendee(a, []string{OfAddress(m.seat)}) {
					outside++
				}
			}
			if internal := captureCallsInternal(parties, m.seat, m.attendees); internal != (outside == 0) {
				t.Errorf("trusted %v, seat %s, attendees %v: capture calls it internal = %v, deal evidence counts %d outside",
					parties.Trusted.Domains(), m.seat, m.attendees, internal, outside)
			}
		}
	}
}
