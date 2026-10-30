// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// captured_by on a note an agent logged names only the passport. Every read of
// the note names the colleague the agent worked for, and the tool, from its
// create audit row. It must say what the record History says about that row.

import (
	"context"
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
	note := s.agentNote(t, link)

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

	for read, row := range s.everyReadOf(s.reader(), t, link.EntityID, note) {
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
		if who.AgentClient == nil || *who.AgentClient != "Claude" {
			t.Errorf("%s: created_by.agent_client = %v, want the passport's client Claude", read, who.AgentClient)
		}
	}
}

// Whose agent wrote a private note, and through which tool, is content: a
// reader outside the note's audience gets the withheld row and no creator.
func TestAWithheldAgentNoteNamesNoCreator(t *testing.T) {
	s := setupStanding(t)
	link := s.contactAt(t, "Anna Weber", "")
	note := s.agentNote(t, link)
	author := s.As(s.Rep1, []ids.UUID{s.Team1}, activityLifecyclePerms)
	if _, err := s.Activities.SetAudience(author, ids.From[ids.ActivityKind](note), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: s.Rep1}},
	}); err != nil {
		t.Fatalf("limiting the note: %v", err)
	}

	for read, row := range s.everyReadOf(s.reader(), t, link.EntityID, note) {
		if row.CreatedBy == nil {
			t.Errorf("%s: the author's own read lost created_by", read)
		}
	}
	outsider := s.As(s.Rep3, []ids.UUID{s.Team2}, roomPerms)
	for read, row := range s.everyReadOf(outsider, t, link.EntityID, note) {
		if row.ContentState == nil || *row.ContentState != crmcontracts.ActivityContentStateWithheld {
			t.Fatalf("%s: the outsider's read is not withheld; the case tests nothing", read)
		}
		if row.CreatedBy != nil {
			t.Errorf("%s: a withheld row names its creator %+v", read, *row.CreatedBy)
		}
	}
}

// agentNote is a note Rep1's agent logged on link, through a passport granted
// to the Claude client.
func (s *standingEnv) agentNote(t *testing.T, link activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	passport := seedGrantedPassport(t, s.Env, s.Rep1, "Claude")
	agent := s.AgentOnPassport(t, s.Rep1, []ids.UUID{s.Team1}, activityLifecyclePerms, ids.From[ids.PassportKind](passport))
	body := "Asked for the renewal terms."
	logged, _, err := s.Activities.LogActivity(agent, activities.LogActivityInput{
		Kind: "note", Body: &body, Links: []activities.ActivityLinkInput{link},
	})
	if err != nil {
		t.Fatalf("the agent's note: %v", err)
	}
	return ids.UUID(logged.Id)
}

// everyReadOf is the note as the activity list, the single read and the
// contact page each answer it.
func (s *standingEnv) everyReadOf(ctx context.Context, t *testing.T, contact, note ids.UUID) map[string]crmcontracts.Activity {
	t.Helper()
	h := s.replyHandlers()
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
