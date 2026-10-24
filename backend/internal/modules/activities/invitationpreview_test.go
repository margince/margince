// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/stretchr/testify/require"

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
			require.Equal(t, tc.wantVideo, appt.VideoCall)
			require.Equal(t, tc.wantLoc, appt.Location)
		})
	}
}
