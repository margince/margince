// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/qualifytest"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadOnMeeting files the meeting on a new lead, owned by owner when one is named.
func leadOnMeeting(t *testing.T, e *integration.SearchEnv, email string, meeting ids.UUID, owner *ids.UUID) ids.UUID {
	t.Helper()
	lead, _, err := contacts.NewStore(e.DB()).CreateLead(e.AsFullUser(), contacts.CreateLeadInput{FullName: new("Lena Prospect"), Email: &email, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Owner.Exec(context.Background(), `UPDATE lead SET owner_id = $1 WHERE id = $2`, owner, lead.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, meeting, lead.Id); err != nil {
		t.Fatal(err)
	}
	return ids.UUID(lead.Id)
}

// A notes Doc landing as a meeting's transcript marks the meeting held. The
// capture writer then asks the lead's owner to qualify the lead, once per sync
// run or more. A lead nobody owns on the same meeting gets no task.
func TestMeetingNotesMarkingALeadMeetingHeldAskTheOwnerToQualify(t *testing.T) {
	t.Parallel()
	env := newCaptureEnvAt(t, clocktest.Now(t))
	e := env.e
	meeting, _, _ := meetingWithNotes(t, env)
	owned := leadOnMeeting(t, e, "lena@prospect.example", meeting, &e.Rep1)
	ownerless := leadOnMeeting(t, e, "milo@prospect.example", meeting, nil)
	_, syncNotes := connectNotes(t, env, &docsFeed{}, captureOwner, &queuedReadings{})

	syncNotes()
	syncNotes()

	if got := notesTranscripts(t, e); got != 1 {
		t.Fatalf("%d transcripts from the notes Doc, want 1", got)
	}
	qualifytest.AssertOwnerAsked(e.AsFullUser(), t, e.DB(), owned, meeting)
	if tasks := qualifytest.OpenTasks(e.AsFullUser(), t, e.DB(), ownerless); len(tasks) != 0 {
		t.Errorf("a lead nobody owns was asked to qualify: %+v", tasks)
	}
}
