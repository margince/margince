// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

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
	t.Parallel()
	f, meeting, _ := reminderFixture(t)
	stager := &recordingStager{}
	for range 2 {
		if err := f.store.SendMeetingReminder(f.ctx, ids.UUID(meeting.Id), stubConsentGate{}, stager); err != nil {
			t.Fatal(err)
		}
	}
	delivery := stager.only(t)
	if !strings.Contains(delivery.Body, "Reschedule or cancel\nhttps://") {
		t.Fatalf("missing management link: %s", delivery.Body)
	}
	var recorded string
	if err := f.env.owner.QueryRow(f.ctx, `SELECT body FROM activity WHERE kind='email'`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(recorded, "/#/book/manage-") || !strings.Contains(recorded, "Reschedule or cancel\n"+redactedMeetingLink) {
		t.Fatalf("the activity any reader fetches must not hold the guest's management link: %s", recorded)
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
	t.Parallel()
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
	t.Parallel()
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

// unheldRecordingStager refuses like recordingStager and keeps each refusal
// offered to it as a background send that was dropped.
type unheldRecordingStager struct {
	recordingStager
	offered []error
}

func (r *unheldRecordingStager) RecordUnheldRefusalTx(_ context.Context, _ pgx.Tx, refusal error) error {
	r.offered = append(r.offered, refusal)
	return nil
}

// A reminder the send rules refuse at staging, such as one whose footer lacks
// the company's address, tells its host why. It is not held for later: a held
// copy could go out after the meeting moved. The job run counts it as handled,
// and the stager is told, so an admin's footer warning counts it.
//
// Mutation: return the refusal from meetingReminderNotSent, or drop the
// recordUnheldRefusalTx call, and this fails.
func TestARefusedMeetingReminderTellsItsHost(t *testing.T) {
	t.Parallel()
	f, meeting, _ := reminderFixture(t)
	told := &attentionRecorder{}
	f.store = f.store.WithInvitationAttention(told)
	stager := &unheldRecordingStager{}
	stager.err = apperrors.ErrConsentNotGranted
	if err := f.store.SendMeetingReminder(f.ctx, ids.UUID(meeting.Id), stubConsentGate{}, stager); err != nil {
		t.Fatalf("a refused reminder failed the job run: %v", err)
	}
	if len(told.reasons) != 1 || told.reasons[0] != AttentionReminderRefused {
		t.Errorf("the host was told %v, want one %s notice", told.reasons, AttentionReminderRefused)
	}
	if len(stager.offered) != 1 || !errors.Is(stager.offered[0], apperrors.ErrConsentNotGranted) {
		t.Errorf("the stager was offered %v, want the one refusal", stager.offered)
	}
	var held int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM scheduled_send WHERE status = 'held'`).Scan(&held); err != nil {
		t.Fatal(err)
	}
	current, err := f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	if held != 0 || current.ReminderStatus == nil || *current.ReminderStatus != "unavailable" {
		t.Errorf("held %d message(s), reminder %v; want none held and the reminder unavailable", held, current.ReminderStatus)
	}
}

func TestMeetingReminderSkipsAMissedWindow(t *testing.T) {
	t.Parallel()
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
