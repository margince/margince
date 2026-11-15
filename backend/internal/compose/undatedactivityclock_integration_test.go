// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An activity logged without a date has happened for every reader at once,
// whichever clock the reader judges by. The app and the database keep their
// own clocks, and they disagree. Here the activities store and the transcript
// reader run on an injected clock that plays the app host's, an hour away from
// the database's now() in either direction.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// appClock is a clock offset from the database's by shift.
func appClock(t *testing.T, shift time.Duration) func() time.Time {
	t.Helper()
	return func() time.Time { return clocktest.Now(t).Add(shift) }
}

// A meeting logged without a date and read at once is a held meeting, even when
// the app's clock is behind the database's. Stamped with the database's now()
// alone, it was in the reader's future and the promise was withheld as
// "meeting not yet held".
func TestAnUndatedMeetingIsHeldForAReaderWhoseClockIsBehindTheDatabase(t *testing.T) {
	e := setupTranscript(t)
	behind := appClock(t, -time.Hour)
	e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
	body, source := "Mira: Priya Raman agreed to send the pricing by Friday.", "transcript"
	meeting, _, err := e.Activities.WithClock(behind).LogActivity(e.ctx, activities.LogActivityInput{
		Kind: "meeting", Body: &body, SourceSystem: &source, Source: "manual",
		AttendeeUserIDs: []ids.UserID{ids.From[ids.UserKind](e.Rep2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.activity = ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	reply, err := json.Marshal(map[string]any{"proposals": []map[string]any{{
		"summary": "Send pricing", "owner": "Priya Raman", "promiser_ref": "seat:" + e.Rep2.String(),
		"source_lines": []int{1}, "due_date": "", "confidence": 0.95, "commitment": openCommitmentMeaning("", ""),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	done := e.readAt(t, cannedBrain{reply: string(reply)}, behind)
	if len(done.ProposalIDs) != 1 {
		t.Fatalf("an undated meeting read by a clock behind the database's staged %d proposals, want one: %v",
			len(done.ProposalIDs), deref(done.StatusDetail))
	}
}

// An undated inbound message is on the waiting list the moment it is logged,
// even when the app's clock is behind the database's.
func TestAnUndatedInboundMessageWaitsForAReplyAtOnceWhenTheAppClockIsBehind(t *testing.T) {
	e := setupTranscript(t)
	store := e.Activities.WithClock(appClock(t, -time.Hour))
	contact := e.SeedContact(t, "Wanda Waiting", &e.Rep1)
	subject, body := "Contract?", "Where is the contract"
	mail, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Direction: new("inbound"), Subject: &subject, Body: &body, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entity := "contact"
	rows, _, err := store.ListActivities(e.Admin(), activities.ListActivitiesInput{
		WaitingReply: true, EntityType: &entity, EntityID: &contact,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Id == mail.Id {
			return
		}
	}
	t.Fatalf("an undated inbound message is missing from the waiting list read by a clock behind the database's (%d rows)", len(rows))
}

// An undated activity counts as the contact's last activity at once, even when
// the app's clock is ahead of the database's. The last-activity clock counts
// only occurred_at <= now(), so a row stamped with the app's clock alone would
// not count until the database caught up.
func TestAnUndatedActivityIsTheLastActivityAtOnceWhenTheAppClockIsAhead(t *testing.T) {
	e := setupTranscript(t)
	contact := e.SeedContact(t, "Arno Ahead", &e.Rep1)
	subject := "Call notes"
	note, _, err := e.Activities.WithClock(appClock(t, time.Hour)).LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	read, err := e.Contacts.GetContact(e.Admin(), ids.From[ids.ContactKind](contact), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.LastActivityAt == nil || !read.LastActivityAt.Equal(note.OccurredAt) {
		t.Fatalf("the contact's last activity is %v, want the undated note at %v", read.LastActivityAt, note.OccurredAt)
	}
}
