// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a free/busy answer rests on, said the same way at both doors.
//
// A window of free slots means two different things depending on where it came
// from: read off a connected calendar, a busy period is one the diary holds;
// derived from this CRM's records, it says nothing about the rest of the day.
// The two are indistinguishable in the answer itself — which is how a reader
// handed a full grid told its owner that a meeting they were having was not on
// their calendar.
//
// The tool door said which it was; the REST door answered the same grid either
// way. This is the claim neither door can make alone, because each is perfectly
// self-consistent while describing a different thing.
//
// Over the RESOLVER both doors are wired to rather than over a database: what
// is under test is that one fact reaches two published answers unchanged, and
// the fact's own derivation from capture's connections is schedulingseam's.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/calendarbacking"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestBothDoorsSayTheSameThingAboutWhatBacksAWindow(t *testing.T) {
	t.Parallel()

	seat := ids.NewV7()
	host := ids.From[ids.UserKind](seat)
	someoneElse := ids.From[ids.UserKind](ids.NewV7())

	for _, c := range []struct {
		name      string
		host      ids.UserID
		connected bool
		want      string
	}{
		{"own host with a calendar", host, true, calendarbacking.Backed},
		{"own host without one", host, false, calendarbacking.Unbacked},
		// Somebody else's: answered without consulting the seam at all, so the
		// reply costs the same whatever that contact has connected.
		{"another seat's host", someoneElse, true, calendarbacking.Unknown},
		{"another seat's host, unconnected", someoneElse, false, calendarbacking.Unknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := principal.WithActor(context.Background(), principal.Principal{
				Type: principal.PrincipalHuman, UserID: seat,
			})
			backs := calendarBacking(func(context.Context, ids.UserID) (bool, error) {
				return c.connected, nil
			})

			// The REST door, through the port the availability handler holds.
			rest, err := activities.CalendarConnected(backs).BackingFor(ctx, seat, c.host)
			if err != nil {
				t.Fatalf("the REST door: %v", err)
			}
			// The tool door, through the adapter check_availability reads.
			tool, err := commsAdapter{calendars: backs}.calendarBackingFor(ctx, c.host)
			if err != nil {
				t.Fatalf("the tool door: %v", err)
			}

			if rest != c.want {
				t.Errorf("the REST door said %q, want %q", rest, c.want)
			}
			if string(tool) != c.want {
				t.Errorf("the tool door said %q, want %q", tool, c.want)
			}
			if rest != string(tool) {
				t.Errorf("the two doors disagree: REST %q, tool %q — a client that asked one "+
					"and a model that asked the other would be told different things about "+
					"the same window", rest, tool)
			}
		})
	}
}
