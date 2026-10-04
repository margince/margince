// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A meeting past the party cap binds the attendees who are seats of this
// workspace and refuses every other name on it — through live capture and
// through the attendee repair alike, so the two cannot split one invitation
// differently.

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// crowdWithSeat is an invitation one past the cap whose guests are strangers
// except for one seat of this workspace.
func crowdWithSeat(seatEmail string) []attendee {
	crowd := []attendee{
		{email: backfillOwner, response: "accepted", self: true},
		{email: seatEmail, response: "accepted"},
	}
	for i := range connector.MaxParticipants {
		crowd = append(crowd, attendee{email: fmt.Sprintf("guest%d@list.example", i), response: "accepted"})
	}
	return crowd
}

// boundOn reads the participant rows of one meeting: which seats it binds, and
// how many rows name somebody who is not a seat.
func boundOn(t *testing.T, e *integration.Env, id ids.UUID) (seats []string, strangers int) {
	t.Helper()
	listed := e.WsScalar(t, `
		SELECT coalesce(string_agg(user_id::text, ',' ORDER BY user_id), '')
		  FROM activity_participant WHERE activity_id = $1 AND user_id IS NOT NULL AND user_id <> $2`,
		id, e.AdminUser)
	if listed != "" {
		seats = strings.Split(listed, ",")
	}
	strangers = e.WsCount(t,
		`SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND user_id IS NULL`, id)
	return seats, strangers
}

// The same crowded invitation, once captured live and once repaired from its
// stored original, binds the same seat and no stranger either way.
func TestACrowdedMeetingBindsItsSeatsAndNoStrangerOnEitherPath(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)
	seatEmail := e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, e.Rep1)
	crowd := crowdWithSeat(seatEmail)

	repaired := seedCapturedMeeting(t, e, "evt-crowd-repair", time.Now().Add(-time.Hour), crowd...)
	if _, err := repairMeetingAttendeesBatch(
		e.Admin(), e.Pool, meetingAttendeeRepairPerTick, slog.Default()); err != nil {
		t.Fatalf("repairMeetingAttendeesBatch: %v", err)
	}

	parties := make([]connector.MessageParticipant, 0, len(crowd))
	for _, a := range crowd[1:] {
		parties = append(parties, connector.MessageParticipant{Email: a.email, Role: connector.ParticipantRoleAttendee})
	}
	capped := connector.CapParticipants(parties)
	if !capped.Capped() {
		t.Fatalf("the fixture names %d parties, which the cap admits — it tests nothing past the cap", capped.Named)
	}
	ref, err := capture.NewSink(e.DB()).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: "evt-crowd-live"},
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "All hands", OccurredAt: time.Now().Add(-time.Hour),
		},
		Source:          calendarSystem + ":evt-crowd-live",
		CapturedBy:      "connector:" + calendarSystem,
		Raw:             calendarPayload(t, "evt-crowd-live", time.Now().Add(-time.Hour), crowd...),
		Participants:    capped.Participants,
		WithheldParties: capped.Withheld,
	}.WithProviderAttestedParticipants(true))
	if err != nil {
		t.Fatalf("capturing the crowded meeting live: %v", err)
	}

	want := []string{e.Rep1.String()}
	for name, id := range map[string]ids.UUID{"repaired": repaired, "live": ref.ID} {
		seats, strangers := boundOn(t, e, id)
		if !slices.Equal(seats, want) {
			t.Errorf("%s meeting binds seats %v, want %v — a colleague on a crowded meeting "+
				"must still be able to read it", name, seats, want)
		}
		if strangers != 0 {
			t.Errorf("%s meeting records %d strangers past the cap, want 0 — a distribution "+
				"list's names are evidence of a list, not of a relationship", name, strangers)
		}
	}
	if outcome, _ := repairOutcome(t, e, repaired); outcome != repairCappedSeats {
		t.Errorf("repair outcome = %q, want %q", outcome, repairCappedSeats)
	}
}

// An unattested list is a sender's text, so past the cap it binds nobody — the
// same refusal the ordinary stamp makes for a Cc line.
func TestAnUnattestedCrowdBindsNoSeat(t *testing.T) {
	e := integration.Setup(t)
	seatEmail := e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, e.Rep1)
	parties := []connector.MessageParticipant{{Email: seatEmail, Role: connector.ParticipantRoleAttendee}}
	for i := range connector.MaxParticipants {
		parties = append(parties, connector.MessageParticipant{
			Email: fmt.Sprintf("guest%d@list.example", i), Role: connector.ParticipantRoleAttendee,
		})
	}
	capped := connector.CapParticipants(parties)
	ref, err := capture.NewSink(e.DB()).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: "evt-unattested"},
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "All hands", OccurredAt: time.Now().Add(-time.Hour),
		},
		Source:          calendarSystem + ":evt-unattested",
		CapturedBy:      "connector:" + calendarSystem,
		WithheldParties: capped.Withheld,
	})
	if err != nil {
		t.Fatalf("capturing the unattested meeting: %v", err)
	}
	if seats, _ := boundOn(t, e, ref.ID); len(seats) != 0 {
		t.Errorf("an unattested crowd bound seats %v, want none", seats)
	}
}

// A meeting an earlier pass settled as `capped` bound nobody, so the repair
// offers it again and settles it under the current rule.
func TestAMeetingStoredAsCappedIsRepairedAgain(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)
	seatEmail := e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, e.Rep1)
	id := seedCapturedMeeting(t, e, "evt-old-capped", time.Now().Add(-time.Hour), crowdWithSeat(seatEmail)...)
	e.WsExec(t, `INSERT INTO activity_meeting_attendee_repair (activity_id, outcome) VALUES ($1, $2)`,
		id, repairCapped)

	if _, err := repairMeetingAttendeesBatch(
		e.Admin(), e.Pool, meetingAttendeeRepairPerTick, slog.Default()); err != nil {
		t.Fatalf("repairMeetingAttendeesBatch: %v", err)
	}
	if outcome, _ := repairOutcome(t, e, id); outcome != repairCappedSeats {
		t.Errorf("outcome = %q, want %q — a meeting left as capped keeps its colleague locked out",
			outcome, repairCappedSeats)
	}
	if seats, _ := boundOn(t, e, id); !slices.Equal(seats, []string{e.Rep1.String()}) {
		t.Errorf("seats = %v, want the colleague on the invitation", seats)
	}
}
