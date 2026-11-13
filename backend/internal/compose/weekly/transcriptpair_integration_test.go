// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A transcript logged as the record of a booked meeting is one meeting in the
// week. It counts once on its day and once held. Its follow-up is that
// meeting's next step.
func TestAMeetingAndItsTranscriptCountOnceInTheWeek(t *testing.T) {
	e := setupWeekly(t)
	now := clocktest.Now(t).UTC()
	booked, subject, start := string(crmcontracts.ActivityMeetingStatusBooked), "Quarterly review", now.Add(-time.Hour)
	meeting, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &start,
		MeetingStatus: &booked, Source: "manual",
	})
	if err != nil {
		t.Fatalf("booking the meeting: %v", err)
	}
	wire := fmt.Sprintf(`{"kind": "meeting", "source": "manual", "source_system": "transcript",
		"subject": "Review call", "body": "Buyer: we renew.", "meeting_status": "held",
		"records_meeting_id": %q, "follow_up_task": {"subject": "Send the quote"}}`, meeting.Id)
	var req crmcontracts.CreateActivityRequest
	if err := json.Unmarshal([]byte(wire), &req); err != nil {
		t.Fatal(err)
	}
	in, err := activities.LogActivityInputFrom(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Activities.LogActivity(e.repCtx, in); err != nil {
		t.Fatalf("logging the transcript: %v", err)
	}

	weekStart := now.Add(-72 * time.Hour)
	weekEnd := weekStart.AddDate(0, 0, daysInWeek)
	err = e.DB().Tx(e.repCtx, func(tx pgx.Tx) error {
		held, withNextStep, err := countWeekMeetings(e.repCtx, tx, e.Rep1, weekStart, weekEnd)
		if err != nil {
			return err
		}
		if held != 1 || withNextStep != 1 {
			t.Errorf("held %d with a next step %d, want 1 and 1", held, withNextStep)
		}
		days, err := countWeekDays(e.repCtx, tx, ids.UUID(e.Rep1), weekStart, weekEnd)
		if err != nil {
			return err
		}
		charted := 0
		for _, day := range days {
			charted += day.MeetingsHeld
		}
		if charted != 1 {
			t.Errorf("the week charts %d meetings, want the one", charted)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Notes typed against a task, with no source_system and no outcome, are a
// meeting of their own. The test for a transcript pair must not drop them.
func TestAMeetingAnsweringATaskWithNoSourceSystemCountsInTheWeek(t *testing.T) {
	e := setupWeekly(t)
	now := clocktest.Now(t).UTC()
	taskSubject, notes, start := "Call the buyer", "Buyer wants a demo.", now.Add(-time.Hour)
	task, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindTask), Subject: &taskSubject, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the task: %v", err)
	}
	answered := ids.UUID(task.Id)
	meeting, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Body: &notes, OccurredAt: &start,
		SourceActivityID: &answered, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the meeting notes: %v", err)
	}
	if meeting.SourceSystem != nil || meeting.MeetingStatus != nil || meeting.SourceActivityId == nil {
		t.Fatalf("the meeting stores source_system %v, status %v, source %v; want none, none and the task",
			meeting.SourceSystem, meeting.MeetingStatus, meeting.SourceActivityId)
	}

	weekStart := now.Add(-72 * time.Hour)
	err = e.DB().Tx(e.repCtx, func(tx pgx.Tx) error {
		days, err := countWeekDays(e.repCtx, tx, ids.UUID(e.Rep1), weekStart, weekStart.AddDate(0, 0, daysInWeek))
		if err != nil {
			return err
		}
		charted := 0
		for _, day := range days {
			charted += day.MeetingsHeld
		}
		if charted != 1 {
			t.Errorf("the week charts %d meetings, want the notes' one", charted)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
