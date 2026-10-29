// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Only the calendar invite's own description asks an attendee to confirm again.
// A note typed into the meeting here, or a resync of the same invite, never does.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const agendaEventID = "evt-agenda"

func agendaInvite(t *testing.T, e *integration.Env, description string) {
	t.Helper()
	organizer := seatEmail(t, e.Rep1)
	ev := meetingmap.Event{
		ID: agendaEventID, Subject: "Pricing review", Description: description,
		StartsAt: laneDay.Add(2 * time.Hour), EndsAt: laneDay.Add(3 * time.Hour),
		Organizer: meetingmap.Actor{Email: organizer}, Attendees: []meetingmap.Actor{{Email: "buyer@customer.test"}},
	}
	record := meetingmap.Classify(ev, organizer).ToRecord(integration.CalendarSystem, []byte(`{"id":"`+agendaEventID+`"}`))
	if _, err := movingCalendarSink(e).Upsert(calendarOwnerCtx(e, e.Rep1), record); err != nil {
		t.Fatalf("capturing the invite: %v", err)
	}
}

func agendaPreparation(t *testing.T, ctx context.Context, store *activities.Store, id ids.UUID) activities.MeetingPreparation {
	t.Helper()
	read, err := store.MeetingPreparations(ctx, []ids.UUID{id})
	if err != nil {
		t.Fatalf("reading the preparation: %v", err)
	}
	got, ok := read[id]
	if !ok {
		t.Fatal("the organizer has no preparation for their own meeting")
	}
	return got
}

func TestOnlyTheInvitesOwnDescriptionAsksTheAttendeeToConfirmAgain(t *testing.T) {
	e := integration.Setup(t)
	agendaInvite(t, e, "Agenda: pricing")
	var id ids.UUID
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT id FROM activity WHERE source_id = $1`, agendaEventID).Scan(&id); err != nil {
		t.Fatalf("finding the captured meeting: %v", err)
	}
	rep := e.As(e.Rep1, nil, integration.AdminPerms)
	store := activities.NewStore(InstallationDB(e.Pool))
	meeting := ids.From[ids.ActivityKind](id)
	if _, err := store.ConfirmMeetingReady(rep, meeting, agendaPreparation(t, rep, store, id).Revision); err != nil {
		t.Fatalf("confirming: %v", err)
	}

	note := "Bring the volume discount sheet."
	if _, err := store.UpdateActivity(rep, meeting, activities.UpdateActivityInput{Body: &note}); err != nil {
		t.Fatalf("typing a note into the meeting: %v", err)
	}
	if got := agendaPreparation(t, rep, store, id); !got.Ready {
		t.Errorf("the rep's own note asked them to confirm again: %+v", got)
	}
	agendaInvite(t, e, "Agenda: pricing")
	if got := agendaPreparation(t, rep, store, id); !got.Ready {
		t.Errorf("a resync of the same invite asked them to confirm again: %+v", got)
	}

	agendaInvite(t, e, "Agenda: pricing and the renewal date")
	got := agendaPreparation(t, rep, store, id)
	if got.Ready || !slices.Equal(got.Changed, []activities.PreparationFact{activities.FactAgenda}) {
		t.Errorf("after the organizer changed the invite the rep reads %+v, want needs review naming only the agenda", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1::text
		AND envelope->'payload'->'changed_fields'->>'agenda_changed' = 'true'`, id); n != 1 {
		t.Errorf("the organizer's change announced %d agenda events, want one", n)
	}
}
