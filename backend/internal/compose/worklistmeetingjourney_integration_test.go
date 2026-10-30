// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The hour before a meeting, on a pinned morning. The meeting leads the rep's
// other work until the rep says I'm ready. An agenda, a link and an opened
// brief do not say it for them.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// journeyMorning is the instant the rep reads their day, well inside it.
var journeyMorning = time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)

func readJourneyDay(ctx context.Context, t *testing.T, e *integration.Env) crmcontracts.Worklist {
	t.Helper()
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return journeyMorning })
	day, err := feed.WorklistView(ctx, "today", "mine", "", ids.Nil, 25, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	return day
}

// meetingRow answers the meeting's row and where it stands, or -1.
func meetingRow(day crmcontracts.Worklist, id ids.UUID) (crmcontracts.WorklistItem, int) {
	for i, row := range day.Queue {
		if row.Source == crmcontracts.WorklistItemSourceMeeting && row.Id == id.String() {
			return row, i
		}
	}
	return crmcontracts.WorklistItem{}, -1
}

func saysStartingSoon(row crmcontracts.WorklistItem) bool {
	for _, because := range row.Because {
		if because.Kind == "meeting_soon" {
			return true
		}
	}
	return false
}

func logJourneyActivity(ctx context.Context, t *testing.T, e *integration.Env, in activities.LogActivityInput) ids.UUID {
	t.Helper()
	in.Source = "manual"
	out, _, err := e.Activities.LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("logging %s %q: %v", in.Kind, *in.Subject, err)
	}
	return ids.UUID(out.Id)
}

func TestAMeetingInAnHourLeadsTheRepsWorkUntilTheRepSaysReady(t *testing.T) {
	e := integration.Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	contact := e.SeedContact(t, "Ana Roth", &e.Rep1)
	assignee := ids.From[ids.UserKind](e.Rep1)
	for i := range 3 {
		subject, due := fmt.Sprintf("Overdue follow-up %d", i), journeyMorning.Add(-time.Duration(i+1)*time.Hour)
		logJourneyActivity(rep, t, e, activities.LogActivityInput{Kind: "task", Subject: &subject, DueAt: &due, AssigneeID: &assignee})
	}
	subject, booked, start := "Pricing review", string(crmcontracts.ActivityMeetingStatusBooked), journeyMorning.Add(time.Hour)
	agenda := "Agenda: renewal terms and the volume discount. Deck: https://deck.example.test/pricing"
	meeting := logJourneyActivity(rep, t, e, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, Body: &agenda, OccurredAt: &start, MeetingStatus: &booked,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if _, err := newMeetingBriefService(e.DB()).Get(rep, meeting); err != nil {
		t.Fatalf("opening the meeting brief: %v", err)
	}

	before := readJourneyDay(rep, t, e)
	lead, at := meetingRow(before, meeting)
	if at != 0 {
		t.Fatalf("the meeting stands at %d of %d rows, want it to lead the overdue tasks", at, len(before.Queue))
	}
	if lead.Preparation == nil || lead.Preparation.State != crmcontracts.MeetingPreparationStateNeedsReview {
		t.Fatalf("after an agenda, a link and an opened brief the meeting reads %+v, want still to prepare", lead.Preparation)
	}

	store := activities.NewStore(InstallationDB(e.Pool))
	if _, err := store.ConfirmMeetingReady(rep, ids.From[ids.ActivityKind](meeting), lead.Preparation.Revision); err != nil {
		t.Fatalf("saying I'm ready: %v", err)
	}

	after := readJourneyDay(rep, t, e)
	reminder, at := meetingRow(after, meeting)
	if at < 0 || !saysStartingSoon(reminder) {
		t.Fatalf("the ready meeting left the list (at %d) or lost its starting-soon reminder: %v", at, reminder.Because)
	}
	if at == 0 || reminder.Level <= lead.Level {
		t.Errorf("the ready meeting still leads at level %d (was %d), want it below the urgent work", reminder.Level, lead.Level)
	}
	if after.Summary.Urgent != before.Summary.Urgent-1 {
		t.Errorf("urgent work went from %d to %d, want exactly one action fewer", before.Summary.Urgent, after.Summary.Urgent)
	}
}
