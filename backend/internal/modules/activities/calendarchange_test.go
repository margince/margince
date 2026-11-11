// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/calendarevent"
)

func hourOf(hour int) *time.Time {
	at := time.Date(2026, 11, 14, hour, 0, 0, 0, time.UTC)
	return &at
}

func sentMeeting(sequence int) calendarevent.Event {
	return calendarevent.Event{
		Sequence: sequence, Title: "Pricing review", Location: "Teams",
		Start: hourOf(14), End: hourOf(15),
		Attendees: []calendarevent.Attendee{{Email: "anna@partner.example", Name: "Anna Weber"}, {Email: "owner@myco.com"}},
	}
}

func TestAFirstInvitationTellsNoChange(t *testing.T) {
	t.Parallel()
	if got := (calendarSend{method: "REQUEST", event: sentMeeting(0)}).change(); got != nil {
		t.Errorf("change = %+v, want none: a first invitation asks for an answer and nothing more", got)
	}
	resent := calendarSend{method: "REQUEST", event: sentMeeting(1), before: new(sentMeeting(1))}
	if got := resent.change(); got != nil {
		t.Errorf("change = %+v, want none for a resend at the same revision", got)
	}
}

func TestAnUpdateSaysWhatMovedAndFromWhere(t *testing.T) {
	t.Parallel()
	moved := sentMeeting(1)
	moved.Start, moved.End, moved.Location = hourOf(16), hourOf(17), "Office Berlin"
	moved.Attendees = append(moved.Attendees, calendarevent.Attendee{Email: "ben@partner.example", Name: "Ben Ito"})
	got := (calendarSend{method: "REQUEST", event: moved, before: new(sentMeeting(0))}).change()
	if got == nil || got.Kind != crmcontracts.EmailCalendarChangeKindUpdated {
		t.Fatalf("change = %+v, want updated", got)
	}
	if !slices.Equal(got.Changed, []crmcontracts.EmailCalendarChangeChanged{"time", "location"}) {
		t.Errorf("changed = %v, want time and location", got.Changed)
	}
	if got.Before == nil || !got.Before.StartsAt.Equal(*hourOf(14)) || *got.Before.Location != "Teams" || got.Before.Title != nil {
		t.Errorf("before = %+v, want 14:00 at Teams and no title", got.Before)
	}
	if !got.StartsAt.Equal(*hourOf(16)) || *got.Location != "Office Berlin" {
		t.Errorf("now = %v at %v, want 16:00 at Office Berlin", got.StartsAt, got.Location)
	}
	if got.GuestsAdded == nil || (*got.GuestsAdded)[0].Email != "ben@partner.example" || got.GuestsRemoved != nil {
		t.Errorf("guests added %v, removed %v; want Ben added", got.GuestsAdded, got.GuestsRemoved)
	}
}

// With no earlier send in the mailbox, a raised revision is still an update.
// One moved meeting of a series is read against its original start.
func TestAnUpdateWithoutAnEarlierSend(t *testing.T) {
	t.Parallel()
	got := (calendarSend{method: "REQUEST", event: sentMeeting(2)}).change()
	if got == nil || got.Kind != crmcontracts.EmailCalendarChangeKindUpdated || len(got.Changed) != 0 {
		t.Errorf("change = %+v, want updated with nothing named", got)
	}
	occurrence := sentMeeting(0)
	occurrence.Occurrence = hourOf(10).Format(time.RFC3339)
	moved := (calendarSend{method: "REQUEST", event: occurrence}).change()
	if moved == nil || !slices.Equal(moved.Changed, []crmcontracts.EmailCalendarChangeChanged{"time"}) ||
		!moved.Before.StartsAt.Equal(*hourOf(10)) {
		t.Errorf("change = %+v, want the time moved from 10:00", moved)
	}
}

func TestACancellationIsCanceledWhateverItsMethod(t *testing.T) {
	t.Parallel()
	statusOnly := sentMeeting(3)
	statusOnly.Canceled = true
	for name, send := range map[string]calendarSend{
		"CANCEL":                    {method: "CANCEL", event: sentMeeting(3)},
		"REQUEST, STATUS:CANCELLED": {method: "REQUEST", event: statusOnly},
	} {
		got := send.change()
		if got == nil || got.Kind != crmcontracts.EmailCalendarChangeKindCanceled || *got.Title != "Pricing review" {
			t.Errorf("%s: change = %+v, want canceled, naming the meeting", name, got)
		}
	}
}

func TestAGuestsProposalAndAnswerNameTheGuest(t *testing.T) {
	t.Parallel()
	proposal := sentMeeting(0)
	proposal.Start, proposal.End = hourOf(9), hourOf(10)
	proposal.Attendees = proposal.Attendees[:1]
	got := (calendarSend{method: "COUNTER", event: proposal, before: new(sentMeeting(0))}).change()
	if got == nil || got.Kind != crmcontracts.EmailCalendarChangeKindProposed || got.By.Email != "anna@partner.example" ||
		!got.StartsAt.Equal(*hourOf(9)) || !got.Before.StartsAt.Equal(*hourOf(14)) {
		t.Errorf("change = %+v, want Anna proposing 09:00 instead of 14:00", got)
	}
	reply := sentMeeting(0)
	reply.Attendees = []calendarevent.Attendee{{Email: "anna@partner.example", Name: "Anna Weber", Answer: "declined"}}
	answered := (calendarSend{method: "REPLY", event: reply}).change()
	if answered == nil || answered.Kind != crmcontracts.EmailCalendarChangeKindAnswered ||
		*answered.Answer != crmcontracts.EmailCalendarChangeAnswerDeclined || *answered.By.Name != "Anna Weber" {
		t.Errorf("change = %+v, want Anna Weber declined", answered)
	}
	silent := calendarSend{method: "REPLY", event: sentMeeting(0)}
	if got := silent.change(); got != nil {
		t.Errorf("change = %+v, want none for a reply that states no answer", got)
	}
}
