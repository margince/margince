// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A decline is one engine behind two doors. The route and the agent's tool
// each take the deal off one of two equal activities, and answer alike. A
// second decline of the same deal writes nothing.
func TestADealDeclineAnswersAsItsRouteThroughTheTool(t *testing.T) {
	d := newTwoDoors(t, "deal-decline-doors", "read", "write")
	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Decline Works", "source": "manual"})
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	deal := createdID(t, d.e, "/v1/deals", AnyMap{
		"name": "Decline renewal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"company_id": company, "source": "manual",
	})
	onDeal := func(subject string) string {
		return createdID(t, d.e, "/v1/activities", AnyMap{
			"kind": "note", "subject": subject, "links": []AnyMap{{"entity_type": "deal", "entity_id": deal}},
		})
	}
	first, second := onDeal("Invoice run"), onDeal("Office party")
	body := AnyMap{"deal_id": deal}
	type answer struct {
		DeclinedDealIDs []string `json:"declined_deal_ids"`
		Unlinked        bool     `json:"unlinked"`
		ProjectUnfiled  bool     `json:"project_unfiled"`
		QuestionsClosed int      `json:"questions_closed"`
	}
	got := sameRecords[answer](t, "decline_deal",
		d.rest(t, "POST", "/v1/activities/"+first+"/deal-decline", body),
		d.tool(t, "change_activities", map[string]any{"action": "decline_deal", "id": second, "request_body": body}))
	if !got.Unlinked || len(got.DeclinedDealIDs) != 1 || got.DeclinedDealIDs[0] != deal {
		t.Fatalf("the decline answered %+v, want deal %s taken off", got, deal)
	}
	for _, activity := range []string{first, second} {
		var read struct {
			Links []struct {
				EntityType string `json:"entity_type"`
			} `json:"links"`
		}
		if status := d.e.Call(t, "GET", "/v1/activities/"+activity, nil, nil, &read); status != http.StatusOK {
			t.Fatalf("reading %s → %d", activity, status)
		}
		for _, link := range read.Links {
			if link.EntityType == "deal" {
				t.Errorf("%s still carries a deal link after the decline", activity)
			}
		}
	}
	again := sameRecords[answer](t, "decline_deal again",
		d.rest(t, "POST", "/v1/activities/"+first+"/deal-decline", body),
		d.tool(t, "change_activities", map[string]any{"action": "decline_deal", "id": second, "request_body": body}))
	if again.Unlinked || len(again.DeclinedDealIDs) != 0 {
		t.Fatalf("a second decline answered %+v, want nothing written", again)
	}
}
