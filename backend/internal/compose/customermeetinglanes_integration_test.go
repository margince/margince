// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLeadMeetingsKeepTheirCustomerLanesWithoutAContactRoster(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Buyer lead"), Email: new("buyer@lead.example"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	now := clocktest.Now(t)
	store := e.Activities.WithClock(func() time.Time { return now })
	for _, link := range []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(lead.Id)}} {
		in := activities.LogActivityInput{Kind: "meeting", Subject: new("Buyer discussion"), Body: new("Discussed the offer"), Source: "manual", OccurredAt: new(now.Add(-time.Hour)), MeetingStatus: new("booked"), Links: []activities.ActivityLinkInput{link}}
		row, _, err := store.LogActivity(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
		row, err = store.GetActivity(e.Admin(), id, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if row.AwaitingOutcome == nil || !*row.AwaitingOutcome {
			t.Fatalf("%s meeting lost its outcome obligation", link.EntityType)
		}
		if _, err := store.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{MeetingStatus: new("held")}); err != nil {
			t.Fatal(err)
		}
		ready, err := store.ReadyForRecap(e.Admin(), id, now)
		if err != nil || !ready {
			t.Fatalf("%s recap=%t: %v", link.EntityType, ready, err)
		}
		in.OccurredAt = new(now.Add(time.Hour))
		future, _, err := store.LogActivity(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		upcoming, err := (attentionMeetings{store: store}).openIn(e.Admin(), attention.MeetingWindow{From: now, Until: now.Add(2 * time.Hour)}, 20, attention.TasksVisible, ids.Nil)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, meeting := range upcoming {
			found = found || meeting.Id == future.Id
		}
		if !found {
			t.Fatalf("%s meeting disappeared from upcoming meetings", link.EntityType)
		}
	}
}
