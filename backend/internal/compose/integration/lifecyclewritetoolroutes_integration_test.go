// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The lifecycle writes: disqualify_lead, promote_lead, demote_lead,
// advance_deal, advance_project_phase and the three relinks. Each test asks both
// doors for the same change on two equal records. It compares the records they
// leave and the audit and outbox rows.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type sharedLead struct {
	Status string `json:"status"`
}

func (d *twoDoors) storedLead(t *testing.T, id string) json.RawMessage {
	t.Helper()
	return d.restStatus(t, "GET", "/v1/leads/"+id, nil, http.StatusOK)
}

// A lead disqualified, promoted and demoted again over either door is left
// alike.
func TestLeadLifecycleWritesLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "lead-write-doors", "read", "write")
	lead := func(name, email string) string {
		return createdID(t, d.e, "/v1/leads", AnyMap{"full_name": name, "email": email, "source": "manual"})
	}

	restLost, toolLost := lead("Rest Lost", "lost-rest@doors.test"), lead("Tool Lost", "lost-tool@doors.test")
	d.restPinned(t, "DELETE", "/v1/leads/"+restLost, AnyMap{}, versionOf(t, d.storedLead(t, restLost)))
	d.tool(t, "disqualify_lead", map[string]any{"lead_id": toolLost})
	sameStored(t, "disqualify", d.storedLead(t, restLost), d.storedLead(t, toolLost), restLost, toolLost, "full_name", "email")
	disqualified := sameRecords[sharedLead](t, "disqualify", d.storedLead(t, restLost), d.storedLead(t, toolLost))
	if disqualified.Status != "disqualified" {
		t.Errorf("the disqualified lead is %q, want disqualified", disqualified.Status)
	}
	d.sameEffect(t, "disqualify", restLost, toolLost)

	restWon, toolWon := lead("Rest Won", "won-rest@doors.test"), lead("Tool Won", "won-tool@doors.test")
	restPromoted := d.restPinned(t, "POST", "/v1/leads/"+restWon+"/promote", AnyMap{"trigger": "human_qualify"},
		versionOf(t, d.storedLead(t, restWon)))
	toolPromoted := d.tool(t, "promote_lead", map[string]any{"lead_id": toolWon, "trigger": "human_qualify"})
	sameStored(t, "promote", d.storedLead(t, restWon), d.storedLead(t, toolWon), restWon, toolWon, "full_name", "email")
	promoted := sameRecords[sharedLead](t, "promote", d.storedLead(t, restWon), d.storedLead(t, toolWon))
	if promoted.Status != "promoted" {
		t.Errorf("the promoted lead is %q, want promoted (route %s, tool %s)", promoted.Status, restPromoted, toolPromoted)
	}
	d.sameEffect(t, "promote", restWon, toolWon)

	d.restPinned(t, "POST", "/v1/leads/"+restWon+"/demote", AnyMap{"reason": "Promoted by mistake"},
		versionOf(t, d.storedLead(t, restWon)))
	d.tool(t, "demote_lead", map[string]any{"lead_id": toolWon, "reason": "Promoted by mistake"})
	sameStored(t, "demote", d.storedLead(t, restWon), d.storedLead(t, toolWon), restWon, toolWon, "full_name", "email")
	demoted := sameRecords[sharedLead](t, "demote", d.storedLead(t, restWon), d.storedLead(t, toolWon))
	if demoted.Status == "promoted" {
		t.Errorf("the demoted lead is still %q", demoted.Status)
	}
	d.sameEffect(t, "demote", restWon, toolWon)
}

type sharedStage struct {
	StageID string `json:"stage_id"`
	Phase   string `json:"phase"`
}

// A deal moved between open stages and a project moved between phases are left
// alike over either door.
func TestAdvanceWritesLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "advance-write-doors", "read", "write")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	next := openStages(t, d.e)[1]
	// One name for both: deal.stage_changed carries it, and the events must match.
	viaREST := openDeal(t, d.e, stages, "Advanced renewal")
	viaTool := openDeal(t, d.e, stages, "Advanced renewal")
	storedDeal := func(id string) json.RawMessage { return d.restStatus(t, "GET", "/v1/deals/"+id, nil, http.StatusOK) }

	d.restPinned(t, "POST", "/v1/deals/"+viaREST+"/advance", AnyMap{"to_stage_id": next}, versionOf(t, storedDeal(viaREST)))
	d.tool(t, "advance_deal", map[string]any{"deal_id": viaTool, "to_stage_id": next, "if_version": versionOf(t, storedDeal(viaTool))})
	sameStored(t, "advance deal", storedDeal(viaREST), storedDeal(viaTool), viaREST, viaTool, "name")
	moved := sameRecords[sharedStage](t, "advance deal", storedDeal(viaREST), storedDeal(viaTool))
	if moved.StageID != next {
		t.Errorf("the advanced deal is at stage %s, want %s", moved.StageID, next)
	}
	d.sameEffect(t, "advance deal", viaREST, viaTool)

	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Phase Doors", "source": "manual"})
	project := func(name string) string {
		return createdID(t, d.e, "/v1/projects", AnyMap{"name": name, "company_id": company, "source": "manual"})
	}
	restProject, toolProject := project("Rest rollout"), project("Tool rollout")
	storedProject := func(id string) json.RawMessage { return d.restStatus(t, "GET", "/v1/projects/"+id, nil, http.StatusOK) }
	d.restPinned(t, "POST", "/v1/projects/"+restProject+"/advance", AnyMap{"to_phase": "pursuing"}, versionOf(t, storedProject(restProject)))
	d.tool(t, "advance_project_phase", map[string]any{"project_id": toolProject, "to_phase": "pursuing", "if_version": versionOf(t, storedProject(toolProject))})
	sameStored(t, "advance project", storedProject(restProject), storedProject(toolProject), restProject, toolProject, "name", "key")
	phased := sameRecords[sharedStage](t, "advance project", storedProject(restProject), storedProject(toolProject))
	if phased.Phase != "pursuing" {
		t.Errorf("the advanced project is in phase %q, want pursuing", phased.Phase)
	}
	d.sameEffect(t, "advance project", restProject, toolProject)
}

// An activity filed under another contact, singly or in a batch, is left alike
// over either door.
func TestRelinkWritesLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "relink-write-doors", "read", "write")
	from := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Relink From", "source": "manual"})
	to := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Relink To", "source": "manual"})
	note := func(subject string) string {
		return createdID(t, d.e, "/v1/activities", AnyMap{
			"kind": "note", "subject": subject, "source": "manual",
			"links": []AnyMap{{"entity_type": "contact", "entity_id": from}},
		})
	}
	stored := func(id string) json.RawMessage {
		return d.restStatus(t, "GET", "/v1/activities/"+id, nil, http.StatusOK)
	}
	linkedTo := func(id string) []string {
		var read struct {
			Links []struct {
				EntityID string `json:"entity_id"`
			} `json:"links"`
		}
		if err := json.Unmarshal(stored(id), &read); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, link := range read.Links {
			out = append(out, link.EntityID)
		}
		return out
	}

	restOne, toolOne := note("Rest filed note"), note("Tool filed note")
	d.restPinned(t, "POST", "/v1/activities/"+restOne+"/relink", AnyMap{"entity_type": "contact", "entity_id": to, "replace_existing_of_type": true},
		versionOf(t, stored(restOne)))
	d.tool(t, "relink_activity", map[string]any{"activity_id": toolOne, "entity_type": "contact", "entity_id": to, "replace_existing_of_type": true})
	for _, id := range []string{restOne, toolOne} {
		if got := linkedTo(id); len(got) != 1 || got[0] != to {
			t.Errorf("activity %s is linked to %v after the relink, want only %s", id, got, to)
		}
	}
	d.sameEffect(t, "relink", restOne, toolOne)

	// A batch runs at once on either door, as the passport's colleague could
	// relink it, and the two doors leave the same rows.
	toRest := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Batch To Rest", "source": "manual"})
	toTool := createdID(t, d.e, "/v1/contacts", AnyMap{"full_name": "Batch To Tool", "source": "manual"})
	restA, toolA := note("Rest batch"), note("Tool batch")
	d.restStatus(t, "POST", "/v1/activities/relink-bulk", AnyMap{
		"activity_ids": []string{restA}, "entity_type": "contact", "entity_id": toRest, "replace_existing_of_type": true,
	}, http.StatusOK)
	d.tool(t, "relink_activities", map[string]any{
		"activity_ids": []string{toolA}, "entity_type": "contact", "entity_id": toTool, "replace_existing_of_type": true,
	})
	if got := linkedTo(restA); len(got) != 1 || got[0] != toRest {
		t.Errorf("activity %s is linked to %v after the batch, want only %s", restA, got, toRest)
	}
	if got := linkedTo(toolA); len(got) != 1 || got[0] != toTool {
		t.Errorf("activity %s is linked to %v after the batch, want only %s", toolA, got, toTool)
	}
	d.sameEffect(t, "relink batch", restA, toolA, "subject")

	// A whole-conversation move stages nothing on either door: both refuse it
	// with one shared refusal.
	var threadRefusal fileRefusal
	restStatus := d.e.Call(t, "POST", "/v1/activities/relink-thread", AnyMap{
		"thread_key": "doors-thread", "entity_type": "contact", "entity_id": toRest,
	}, d.bearer, &threadRefusal)
	if restStatus != http.StatusUnprocessableEntity {
		t.Fatalf("a thread move over REST → %d, want 422", restStatus)
	}
	toolRefusal := d.toolRefused(t, "relink_thread", map[string]any{
		"thread_key": "doors-thread", "entity_type": "contact", "entity_id": toTool,
	})
	// The route shortens a long refusal with an ellipsis, so the shared part is
	// what it says before that.
	said := strings.TrimSuffix(threadRefusal.Detail, "…")
	if said == "" || !strings.Contains(toolRefusal, said) {
		t.Errorf("the route refused a thread move with %d %q and the tool with %q, want one refusal", restStatus, threadRefusal.Detail, toolRefusal)
	}
	if staged := d.stagedOn(t, toRest); len(staged) != 0 {
		t.Errorf("the batch and the thread move left %d approvals on the destination, want none", len(staged))
	}
}
