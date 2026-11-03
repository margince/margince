// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contact page's timeline stamps awaiting_outcome as the activity list does,
// judged at the page's own clock.
func TestTheContactTimelineStampsWhichMeetingsAwaitAnOutcome(t *testing.T) {
	e := integration.Setup(t)
	now := clocktest.Now(t)
	contact := e.SeedContact(t, "Dana Buyer", nil)
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	logged := map[string]bool{}
	subjectOf := map[crmcontracts.Id]string{}
	for subject, meeting := range map[string]struct {
		days int
		owed bool
	}{"Booked last month": {-30, true}, "Booked next week": {7, false}} {
		at := now.AddDate(0, 0, meeting.days)
		out, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &at,
			MeetingStatus: &booked, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		})
		if err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
		logged[subject] = meeting.owed
		subjectOf[out.Id] = subject
	}

	page, err := pageAsOf(e, now).Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil || page.Activities == nil {
		t.Fatalf("assembling the contact page: %v", err)
	}
	seen := 0
	for _, row := range page.Activities.Data {
		subject, ours := subjectOf[row.Id]
		if !ours {
			continue
		}
		seen++
		if row.AwaitingOutcome == nil || *row.AwaitingOutcome != logged[subject] {
			t.Errorf("%q reads awaiting_outcome %v on the contact timeline, want %v", subject, row.AwaitingOutcome, logged[subject])
		}
	}
	if seen != len(logged) {
		t.Fatalf("the contact timeline holds %d of the %d meetings, so the stamp went unread", seen, len(logged))
	}
}
