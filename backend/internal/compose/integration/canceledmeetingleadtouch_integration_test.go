// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A called-off meeting is no touch in a lead's last activity, which is derived
// from the lead's links rather than stored. Meetings are logged with
// LogActivity and called off with UpdateActivity.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func logTouchMeeting(t *testing.T, e *Env, on activities.ActivityLinkInput, status string, at time.Time) ids.ActivityID {
	t.Helper()
	subject := "Touch meeting"
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, MeetingStatus: &status, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{on},
	})
	if err != nil {
		t.Fatalf("logging the %s meeting: %v", status, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

func callOff(t *testing.T, e *Env, meeting ids.ActivityID, status string) {
	t.Helper()
	if _, err := e.Activities.UpdateActivity(e.Admin(), meeting, activities.UpdateActivityInput{MeetingStatus: &status}); err != nil {
		t.Fatalf("marking the meeting %s: %v", status, err)
	}
}

func TestANoShowIsNotALeadsLastActivity(t *testing.T) {
	e := Setup(t)
	now := time.Now().UTC().Truncate(time.Second)
	name := "Touch Lead"
	created, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Source: "manual"})
	if err != nil {
		t.Fatalf("creating the lead: %v", err)
	}
	lead := ids.UUID(created.Id)
	on := activities.ActivityLinkInput{EntityType: "lead", EntityID: lead}
	heldAt, missedAt := now.AddDate(0, 0, -10), now.AddDate(0, 0, -2)
	logTouchMeeting(t, e, on, "held", heldAt)
	missed := logTouchMeeting(t, e, on, "booked", missedAt)

	lastActivity := func() *time.Time {
		l, err := e.Contacts.GetLead(e.Admin(), ids.From[ids.LeadKind](lead), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		return l.LastActivityAt
	}
	if got := lastActivity(); got == nil || !got.Equal(missedAt) {
		t.Fatalf("lead last activity = %v before the no-show, want %v — the case below proves nothing", got, missedAt)
	}
	callOff(t, e, missed, "no_show")
	if got := lastActivity(); got == nil || !got.Equal(heldAt) {
		t.Errorf("lead last activity = %v, want the held meeting %v — a no-show is not a touch", got, heldAt)
	}
}
