// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The hour before a meeting, read through the routes the screen uses. The
// meeting leads the rep's other work until the rep says I'm ready. An agenda,
// a link and an opened brief do not say it for them.

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func readOwnWorklist(t *testing.T, e *apptest.AppEnv) crmcontracts.Worklist {
	t.Helper()
	var day crmcontracts.Worklist
	if status := e.Call(t, http.MethodGet, "/v1/worklist?scope=mine&filter=all", nil, nil, &day); status != http.StatusOK {
		t.Fatalf("reading the Worklist → %d", status)
	}
	return day
}

// queueRow answers the row of this source and id, and where it stands.
func queueRow(day crmcontracts.Worklist, source crmcontracts.WorklistItemSource, id string) (crmcontracts.WorklistItem, int) {
	for i, row := range day.Queue {
		if row.Source == source && row.Id == id {
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

func TestAMeetingInAnHourLeadsTheRepsWorkUntilTheRepSaysReady(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Meeting Journey", "journey@fable.test", "Admin")
	now := clocktest.Now(t).UTC()
	contact := createdID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Ana Roth"})
	for i := range 3 {
		createdID(t, e, "/v1/activities", AnyMap{
			"kind": "task", "subject": fmt.Sprintf("Overdue follow-up %d", i),
			"due_at": now.Add(-time.Duration(i+1) * time.Hour).Format(time.RFC3339),
		})
	}
	meeting := createdID(t, e, "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Pricing review", "source": "manual", "meeting_status": "booked",
		"occurred_at": now.Add(time.Hour).Truncate(time.Minute).Format(time.RFC3339),
		"body":        "Agenda: renewal terms and the volume discount. Deck: https://deck.example.test/pricing",
		"links":       []AnyMap{{"entity_type": "contact", "entity_id": contact}},
	})
	if status := e.Call(t, http.MethodGet, "/v1/activities/"+meeting+"/meeting-brief", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("opening the meeting brief → %d", status)
	}

	before := readOwnWorklist(t, e)
	lead, at := queueRow(before, crmcontracts.WorklistItemSourceMeeting, meeting)
	if at != 0 {
		t.Fatalf("the meeting stands at %d of %d rows, want it to lead the overdue tasks", at, len(before.Queue))
	}
	if lead.Preparation == nil || lead.Preparation.State != crmcontracts.MeetingPreparationStateNeedsReview {
		t.Fatalf("after an agenda, a link and an opened brief the meeting reads %+v, want still to prepare", lead.Preparation)
	}

	var confirmed crmcontracts.MeetingPreparation
	if status := e.Call(t, http.MethodPut, "/v1/worklist/ready", AnyMap{
		"activity_id": meeting, "preparation_revision": lead.Preparation.Revision,
	}, nil, &confirmed); status != http.StatusOK || confirmed.State != crmcontracts.MeetingPreparationStateReady {
		t.Fatalf("saying I'm ready → %d %+v", status, confirmed)
	}

	after := readOwnWorklist(t, e)
	reminder, at := queueRow(after, crmcontracts.WorklistItemSourceMeeting, meeting)
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
