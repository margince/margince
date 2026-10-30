// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// captured_by on a note an agent logged names only the passport. Every read of
// the note names the colleague the agent worked for, from its create audit row.
// It must say what the record History says about that same row.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryReadOfAnAgentLoggedNoteNamesTheColleagueItWorkedFor(t *testing.T) {
	s := setupStanding(t)
	link := s.contactAt(t, "Anna Weber", "")
	body := "Asked for the renewal terms."
	agent := s.AgentFor(t, s.Rep1, []ids.UUID{s.Team1}, activityLifecyclePerms)
	logged, _, err := s.Activities.LogActivity(agent, activities.LogActivityInput{
		Kind: "note", Body: &body, Links: []activities.ActivityLinkInput{link},
	})
	if err != nil {
		t.Fatalf("the agent's note: %v", err)
	}
	note := ids.UUID(logged.Id)

	recorded, err := privacy.ListRecordHistory(s.Admin(), s.DB(), privacy.RecordHistoryFilter{EntityType: "activity", EntityID: note})
	if err != nil {
		t.Fatalf("reading the note's history: %v", err)
	}
	var history *string
	for _, entry := range recorded.Entries {
		if entry.Action == "create" {
			history = entry.OnBehalfOfName
		}
	}
	if history == nil || *history == "" {
		t.Fatal("the record history names no colleague behind the note's create")
	}

	for read, row := range s.everyReadOf(t, link.EntityID, note) {
		who := row.CreatedBy
		if who == nil {
			t.Errorf("%s: created_by is absent for a note whose create row exists", read)
			continue
		}
		if who.ActorType != crmcontracts.PrincipalAttributionActorTypeAgent {
			t.Errorf("%s: created_by.actor_type = %q, want agent", read, who.ActorType)
		}
		if who.OnBehalfOfName == nil || *who.OnBehalfOfName != *history {
			t.Errorf("%s: created_by names %v, the record history names %q", read, who.OnBehalfOfName, *history)
		}
	}
}

// everyReadOf is the note as the activity list, the single read and the
// contact page each answer it.
func (s *standingEnv) everyReadOf(t *testing.T, contact, note ids.UUID) map[string]crmcontracts.Activity {
	t.Helper()
	ctx, h := s.reader(), s.replyHandlers()
	out := map[string]crmcontracts.Activity{}
	code, body := serve(ctx, t, func(w http.ResponseWriter, r *http.Request) {
		h.ListActivities(w, r, crmcontracts.ListActivitiesParams{})
	})
	if code != http.StatusOK {
		t.Fatalf("GET /activities answered %d", code)
	}
	for _, a := range readBody[activityPage](t, body).Data {
		if ids.UUID(a.Id) == note {
			out["GET /activities"] = a
		}
	}
	code, body = serve(ctx, t, func(w http.ResponseWriter, r *http.Request) { h.GetActivity(w, r, crmcontracts.Id(note)) })
	if code != http.StatusOK {
		t.Fatalf("GET /activities/{id} answered %d", code)
	}
	out["GET /activities/{id}"] = readBody[crmcontracts.Activity](t, body)
	page := contact360.NewHandlers(contactRoomService(s.Env))
	code, body = serve(ctx, t, func(w http.ResponseWriter, r *http.Request) {
		page.GetContact360(w, r, crmcontracts.Id(contact), crmcontracts.GetContact360Params{})
	})
	if code != http.StatusOK {
		t.Fatalf("GET /contacts/{id}/360 answered %d", code)
	}
	if timeline := readBody[crmcontracts.Contact360](t, body).Activities; timeline != nil {
		for _, a := range timeline.Data {
			if ids.UUID(a.Id) == note {
				out["GET /contacts/{id}/360"] = a
			}
		}
	}
	if len(out) != 3 {
		t.Fatalf("the note is missing from a read: found it on %d of 3", len(out))
	}
	return out
}
