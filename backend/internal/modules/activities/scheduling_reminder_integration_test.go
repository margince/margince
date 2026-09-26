// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type reminderCalendar struct {
	*invitationCalendar
	state connector.CalendarState
}

func (c *reminderCalendar) Inspect(context.Context, ids.UserID, string, string, string) (connector.CalendarState, error) {
	return c.state, nil
}

func reminderFixture(t *testing.T) (*invitationFixture, crmcontracts.MeetingInvitation, *reminderCalendar) {
	t.Helper()
	f := newInvitationFixture(t)
	profile, err := f.store.SchedulingProfile(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	profile.EmailReminder = &enabled
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	calendar := &reminderCalendar{invitationCalendar: f.calendar, state: connector.CalendarState{Start: f.request.Start, End: f.request.End}}
	f.store = f.store.WithSchedulingCalendar(calendar).WithRecipientDirectory(knownRecipients{})
	meeting := f.book(t)
	f.now = monday(9)
	return f, meeting, calendar
}

func TestMeetingReminderQueuesOnceThroughTheEmailWriter(t *testing.T) {
	f, meeting, _ := reminderFixture(t)
	stager := &recordingStager{}
	for range 2 {
		if err := f.store.SendMeetingReminder(f.ctx, ids.UUID(meeting.Id), stubConsentGate{}, stager); err != nil {
			t.Fatal(err)
		}
	}
	delivery := stager.only(t)
	if !strings.Contains(delivery.Body, "Reschedule or cancel:") {
		t.Fatalf("missing management link: %s", delivery.Body)
	}
	current, err := f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	if current.ReminderStatus == nil || *current.ReminderStatus != "queued" {
		t.Fatalf("reminder state: %+v", current)
	}
}

func TestMeetingReminderRefreshesProviderMovesAndCancellations(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "moved", true: "canceled"}[canceled], func(t *testing.T) {
			f, meeting, calendar := reminderFixture(t)
			calendar.state = connector.CalendarState{Canceled: canceled, Start: monday(13), End: monday(14)}
			stager := &recordingStager{}
			if err := f.store.SendMeetingReminder(f.ctx, ids.UUID(meeting.Id), stubConsentGate{}, stager); err != nil {
				t.Fatal(err)
			}
			if len(stager.staged) != 0 {
				t.Fatal("stale reminder queued")
			}
			due, err := f.store.DueMeetingReminders(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(due) != 0 {
				t.Fatalf("old reminder remains due: %+v", due)
			}
		})
	}
}

func TestMeetingReminderRecordsARefusalWithoutSending(t *testing.T) {
	f, meeting, _ := reminderFixture(t)
	stager := &recordingStager{err: apperrors.ErrPermissionDenied}
	err := f.store.SendMeetingReminder(f.ctx, ids.UUID(meeting.Id), stubConsentGate{}, stager)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("lost refusal: %v", err)
	}
	var emails int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM activity WHERE kind='email'`).Scan(&emails); err != nil {
		t.Fatal(err)
	}
	if emails != 0 {
		t.Fatal("refused reminder committed an email")
	}
	current, err := f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	if current.ReminderStatus == nil || *current.ReminderStatus != "unavailable" {
		t.Fatalf("hidden refusal: %+v", current)
	}
}

func TestMeetingReminderSkipsAMissedWindow(t *testing.T) {
	f, meeting, _ := reminderFixture(t)
	f.now = monday(12)
	stager := &recordingStager{}
	if err := f.store.SendMeetingReminder(f.ctx, ids.UUID(meeting.Id), stubConsentGate{}, stager); err != nil {
		t.Fatal(err)
	}
	if len(stager.staged) != 0 {
		t.Fatal("reminded after the meeting")
	}
	due, err := f.store.DueMeetingReminders(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatal("missed reminder keeps waking")
	}
}
