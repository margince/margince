// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

func TestTheBookingLengthUsesOnlyOfferedLengths(t *testing.T) {
	profile := defaultSchedulingProfile()
	profile.OfferedLengths = new([]int{45, 60})
	for _, test := range []struct {
		requested *int
		want      int
	}{{nil, 30}, {new(45), 45}, {new(60), 60}, {new(47), 30}, {new(0), 30}, {new(-30), 30}} {
		if got := bookingLength(profile, test.requested).DurationMinutes; got != test.want {
			t.Errorf("requested %v: got %d, want %d", test.requested, got, test.want)
		}
	}
	if profile.DurationMinutes != 30 {
		t.Fatal("a guest changed the host's default")
	}
}

func TestOfferedMeetingLengthsAreDistinctAndBounded(t *testing.T) {
	for _, lengths := range [][]int{{0}, {481}, {30, 30}} {
		profile := defaultSchedulingProfile()
		profile.OfferedLengths = &lengths
		if validateSchedulingProfile(profile) == nil {
			t.Errorf("accepted invalid lengths %v", lengths)
		}
	}
}
