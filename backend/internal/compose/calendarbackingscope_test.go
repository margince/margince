// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// WHOSE calendar this seat may ask about.
//
// check_availability takes any host_user_id, and whether a colleague has
// connected Google or Microsoft is their account's business: capture is
// per-user, and its own connections reader hard-scopes to the actor for that
// reason. A version of this that answered for an arbitrary host let anyone
// holding `read` walk the roster and learn who has a live grant — and, because
// `connected` flips when a grant errors or awaits re-consent, watch one fail.
//
// The assertion that matters is not the value returned for a foreign host but
// that NOTHING IS READ for one. A version that read first and withheld
// afterwards would answer the same bytes and still be a timing signal, so the
// seam counts its calls and the foreign case must leave the count at zero.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// countingCalendars answers a fixed verdict and records that it was asked.
func countingCalendars(connected bool, asked *int) calendarBacking {
	return func(context.Context, ids.UserID) (bool, error) {
		*asked++
		return connected, nil
	}
}

func actingAs(user ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
	})
}

func TestTheCalendarBackingIsOnlyReadForTheActingSeat(t *testing.T) {
	t.Parallel()
	me, colleague := ids.NewV7(), ids.NewV7()

	for _, tc := range []struct {
		name      string
		host      ids.UUID
		connected bool
		want      agents.CalendarBacking
		wantAsked int
	}{
		{"my own seat, connected", me, true, agents.CalendarBacked, 1},
		{"my own seat, not connected", me, false, agents.CalendarUnbacked, 1},
		{"a colleague's seat", colleague, true, agents.CalendarBackingUnknown, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := 0
			adapter := commsAdapter{calendars: countingCalendars(tc.connected, &asked)}

			got, err := adapter.calendarBackingFor(actingAs(me), ids.From[ids.UserKind](tc.host))
			if err != nil {
				t.Fatalf("reading the calendar backing: %v", err)
			}
			if got != tc.want {
				t.Errorf("backing = %q, want %q", got, tc.want)
			}
			if asked != tc.wantAsked {
				t.Errorf("the connector was read %d times, want %d — reading it for a host who is "+
					"not the acting seat reports that person's account state", asked, tc.wantAsked)
			}
		})
	}
}

// A principal with no user at all reads nothing either. A scheduled job has a
// zero UserID, and comparing zero against a zero host would otherwise make
// every unattended run look like the host's own seat.
func TestAPrincipalWithNoSeatReadsNoCalendar(t *testing.T) {
	t.Parallel()
	asked := 0
	adapter := commsAdapter{calendars: countingCalendars(true, &asked)}

	got, err := adapter.calendarBackingFor(context.Background(), ids.From[ids.UserKind](ids.UUID{}))
	if err != nil {
		t.Fatalf("reading the calendar backing: %v", err)
	}
	if got != agents.CalendarBackingUnknown {
		t.Errorf("backing = %q, want %q for a principal with no seat", got, agents.CalendarBackingUnknown)
	}
	if asked != 0 {
		t.Errorf("the connector was read %d times for a principal with no seat", asked)
	}
}
