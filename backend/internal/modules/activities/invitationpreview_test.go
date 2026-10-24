// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestInvitationAppointmentDropsTheLocationOfAVideoCall(t *testing.T) {
	on, off := true, false
	loc := "https://meet.google.com/old-link"
	cases := []struct {
		name      string
		profile   *bool
		requested *bool
		wantVideo bool
		wantLoc   string
	}{
		{"profile video on", &on, nil, true, ""},
		{"profile video unset counts as on", nil, nil, true, ""},
		{"request turns video on over a profile that has it off", &off, &on, true, ""},
		{"profile video off keeps the location", &off, nil, false, loc},
		{"request turns video off over a profile that has it on", &on, &off, false, loc},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := crmcontracts.SchedulingProfile{VideoCall: tc.profile, Location: loc}
			appt := invitationAppointment(profile, crmcontracts.MeetingInvitationRequest{Location: loc, VideoCall: tc.requested})
			if appt.VideoCall != tc.wantVideo {
				t.Errorf("VideoCall = %v, want %v", appt.VideoCall, tc.wantVideo)
			}
			if appt.Location != tc.wantLoc {
				t.Errorf("Location = %q, want %q", appt.Location, tc.wantLoc)
			}
		})
	}
}
