// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// organizersSaying answers a fixed set of organizers.
type organizersSaying map[ids.UUID]ids.UUID

func (o organizersSaying) MeetingOrganizers(_ context.Context, meetingIDs []ids.UUID) (map[ids.UUID]ids.UUID, error) {
	out := map[ids.UUID]ids.UUID{}
	for _, id := range meetingIDs {
		if organizer, ok := o[id]; ok {
			out[id] = organizer
		}
	}
	return out, nil
}

func meetingRow(source crmcontracts.WorklistItemSource, meeting, calendarSeat ids.UUID) crmcontracts.WorklistItem {
	seat := crmcontracts.WorklistOwner{Kind: crmcontracts.WorklistOwnerKindWorklistOwnerUser, Id: idPtr(calendarSeat)}
	host := seat
	return crmcontracts.WorklistItem{
		Source:  source,
		Subject: &crmcontracts.AttentionSubject{Type: crmcontracts.AttentionSubjectTypeActivity, Id: openapi_types.UUID(meeting)},
		Owner:   &seat,
		Host:    &host,
	}
}

// A meeting an invited colleague's calendar stored first is hosted by the
// colleague who organized it. The row's owner stays the calendar's seat.
func TestAMeetingRowIsHostedByTheColleagueWhoOrganizedIt(t *testing.T) {
	t.Parallel()
	organizer, invitee := ids.NewV7(), ids.NewV7()
	organized, outsider := ids.NewV7(), ids.NewV7()
	svc := &Service{
		organizers: organizersSaying{organized: organizer},
		teammates:  roster{{UserID: organizer, DisplayName: "Ana Organizer"}, {UserID: invitee, DisplayName: "Ben Invitee"}},
	}
	rows := []crmcontracts.WorklistItem{
		meetingRow(sourceMeeting, organized, invitee),
		meetingRow(sourceMeetingOutcome, organized, invitee),
		meetingRow(sourceMeeting, outsider, invitee),
	}
	if err := svc.nameRows(context.Background(), rows, nil); err != nil {
		t.Fatalf("naming the rows: %v", err)
	}
	for i, want := range []ids.UUID{organizer, organizer, invitee} {
		host := rows[i].Host
		if host == nil || host.Id == nil || ids.UUID(*host.Id) != want {
			t.Errorf("row %d: host = %+v, want %v", i, host, want)
		}
		if rows[i].Owner == nil || ids.UUID(*rows[i].Owner.Id) != invitee {
			t.Errorf("row %d: owner = %+v, want the calendar's seat %v unchanged", i, rows[i].Owner, invitee)
		}
	}
	if label := rows[0].Host.Label; label == nil || *label != "Ana Organizer" {
		t.Errorf("the organizer's host label = %v, want Ana Organizer", label)
	}
}

// A row about something other than a meeting is not asked about.
func TestOnlyAMeetingRowAsksWhoOrganizedIt(t *testing.T) {
	t.Parallel()
	task := ids.NewV7()
	svc := &Service{organizers: organizersSaying{task: ids.NewV7()}}
	rows := []crmcontracts.WorklistItem{{
		Source:  "task",
		Subject: &crmcontracts.AttentionSubject{Type: crmcontracts.AttentionSubjectTypeActivity, Id: openapi_types.UUID(task)},
	}}
	if err := svc.nameTheOrganizers(context.Background(), rows); err != nil {
		t.Fatalf("naming the organizers: %v", err)
	}
	if rows[0].Host != nil {
		t.Errorf("a task row was given a host: %+v", rows[0].Host)
	}
}
