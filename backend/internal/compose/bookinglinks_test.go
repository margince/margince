// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One request shape, two credentials, one answer.
//
// `/bookings` and the `book_meeting` tool are the same operation reached two
// ways, and a booking attached to no record is refused on both. The refusals are
// written in two modules that cannot see each other — activities holds the
// store's, agents holds the staging gate's — so nothing in Go makes them agree,
// and for a while they did not: the tool refused what the contract permitted and
// the REST door wrote it.
//
// Which half was wrong is settled (the contract now carries `minItems: 1`, on
// the reason the account send already gave). What this holds is the property the
// authority model exists for: an agent and a human sending the same request are
// told the same thing, by the same status, naming the same machine field. Only
// compose sees both doors.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestABookingAttachedToNothingIsRefusedTheSameWayAtBothDoors(t *testing.T) {
	t.Parallel()
	host := ids.NewV7()
	ctx := bookingDoorCtx(host)
	start := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)

	// The window is valid and the calendar is the caller's own at both doors,
	// so neither refusal can be the slot or the delegation rule answering in
	// the link rule's place.
	_, restErr := (&activities.Store{}).BookMeeting(ctx, activities.BookMeetingInput{
		Host: ids.From[ids.UserKind](host), Start: start, End: start.Add(time.Hour),
	})
	// A nil record seam is safe and is part of the claim: an empty list is
	// refused before anything is looked up, so the tool door answers without
	// reading a record — which is why the two doors can be compared here at all.
	toolErr := agents.NewBookMeetingCall(nil, agents.BookMeetingCommand{
		Start: start, End: start.Add(time.Hour),
	}).Guards(ctx)

	rest := classifyBookingRefusal(t, "the REST door", restErr)
	tool := classifyBookingRefusal(t, "the tool door", toolErr)

	if rest.Status != tool.Status {
		t.Errorf("the REST door answers %d and the tool door %d for the same request: one caller is "+
			"told to fix their arguments and the other is not", rest.Status, tool.Status)
	}
	if rest.Status != http.StatusUnprocessableEntity {
		t.Errorf("both doors answer %d, want %d — the body was well formed and one required value "+
			"was absent", rest.Status, http.StatusUnprocessableEntity)
	}
	if restField, toolField := soleFaultField(t, rest), soleFaultField(t, tool); restField != toolField {
		t.Errorf("the REST door names %q and the tool door %q: a client branching on `details.errors` "+
			"has to know which door answered it", restField, toolField)
	} else if restField != "links" {
		t.Errorf("both doors name %q, want links — that is the argument the caller must supply", restField)
	}
}

// classifyBookingRefusal insists the refusal reached the taxonomy at all. An
// error outside it is answered as an opaque 500 telling the caller to retry a
// call that will be refused identically forever, which is the failure this
// comparison would otherwise read as agreement.
func classifyBookingRefusal(t *testing.T, door string, err error) httperr.Fault {
	t.Helper()
	if err == nil {
		t.Fatalf("%s accepted a booking attached to no record: it lands on no timeline, and nobody "+
			"reaches it again except by already knowing it exists", door)
	}
	fault, ok := httperr.Classify(err)
	if !ok {
		t.Fatalf("%s refuses with %v, which is outside the taxonomy — the caller's own mistake is "+
			"reported as an internal server fault", door, err)
	}
	return fault
}

func soleFaultField(t *testing.T, fault httperr.Fault) string {
	t.Helper()
	if len(fault.Fields) != 1 {
		t.Fatalf("the refusal carries %d field entries, want exactly 1: %+v", len(fault.Fields), fault.Fields)
	}
	return fault.Fields[0].Field
}

// bookingDoorCtx is one principal both doors accept: a human who may create
// activities, booking their OWN calendar. Both doors settle authority and
// delegation before they look at the links, so without either half the refusal
// under test never runs and the comparison would be about permissions instead.
func bookingDoorCtx(userID ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:     principal.PrincipalHuman,
		ID:       "human:" + userID.String(),
		UserID:   userID,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Create: true}},
			RowScope: principal.RowScopeOwn,
		},
	})
}
