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
// each mark one of two equal activities not about the deal, and answer alike.
// The link stays, carrying the mark. A second decline writes nothing.
func TestADealDeclineAnswersAsItsRouteThroughTheTool(t *testing.T) {
	d, deal, first, second := newMarkedDoors(t, "deal-decline-doors")
	body := AnyMap{"deal_id": deal}
	type answer struct {
		DeclinedDealIDs []string `json:"declined_deal_ids"`
		Unlinked        bool     `json:"unlinked"`
		MarkedUnrelated bool     `json:"marked_unrelated"`
		ProjectUnfiled  bool     `json:"project_unfiled"`
		QuestionsClosed int      `json:"questions_closed"`
	}
	got := sameRecords[answer](t, "decline_deal",
		d.rest(t, "POST", "/v1/activities/"+first+"/deal-decline", body),
		d.tool(t, "change_activities", map[string]any{"action": "decline_deal", "id": second, "request_body": body}))
	if !got.MarkedUnrelated || got.Unlinked || len(got.DeclinedDealIDs) != 1 || got.DeclinedDealIDs[0] != deal {
		t.Fatalf("the decline answered %+v, want deal %s marked", got, deal)
	}
	for _, activity := range []string{first, second} {
		wantDealLinkMark(t, d, activity, deal, "unrelated")
	}
	again := sameRecords[answer](t, "decline_deal again",
		d.rest(t, "POST", "/v1/activities/"+first+"/deal-decline", body),
		d.tool(t, "change_activities", map[string]any{"action": "decline_deal", "id": second, "request_body": body}))
	if again.MarkedUnrelated || len(again.DeclinedDealIDs) != 0 {
		t.Fatalf("a second decline answered %+v, want nothing written", again)
	}
}

// The mark is one engine behind two doors too, and "relevant" is its undo.
func TestADealMarkAnswersAsItsRouteThroughTheTool(t *testing.T) {
	d, deal, first, second := newMarkedDoors(t, "deal-relevance-doors")
	type answer struct {
		DealRelevance   string `json:"deal_relevance"`
		RelevanceSource string `json:"relevance_source"`
		Changed         bool   `json:"changed"`
	}
	for _, relevance := range []string{"unrelated", "relevant"} {
		body := AnyMap{"deal_id": deal, "deal_relevance": relevance}
		got := sameRecords[answer](t, "deal_relevance "+relevance,
			d.rest(t, "PUT", "/v1/activities/"+first+"/deal-relevance", body),
			d.tool(t, "change_activities", map[string]any{"action": "deal_relevance", "id": second, "request_body": body}))
		if got != (answer{DealRelevance: relevance, RelevanceSource: "human", Changed: true}) {
			t.Fatalf("marking %s answered %+v", relevance, got)
		}
		for _, activity := range []string{first, second} {
			wantDealLinkMark(t, d, activity, deal, relevance)
		}
	}
}

// newMarkedDoors is a deal and two equal notes on it, behind both doors.
func newMarkedDoors(t *testing.T, slug string) (d *twoDoors, deal, first, second string) {
	t.Helper()
	d = newTwoDoors(t, slug, "read", "write")
	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Decline Works", "source": "manual"})
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	deal = createdID(t, d.e, "/v1/deals", AnyMap{
		"name": "Decline renewal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"company_id": company, "source": "manual",
	})
	onDeal := func(subject string) string {
		return createdID(t, d.e, "/v1/activities", AnyMap{
			"kind": "note", "subject": subject, "links": []AnyMap{{"entity_type": "deal", "entity_id": deal}},
		})
	}
	return d, deal, onDeal("Invoice run"), onDeal("Office party")
}

// wantDealLinkMark reads the activity and fails unless its link to deal
// carries relevance.
func wantDealLinkMark(t *testing.T, d *twoDoors, activity, deal, relevance string) {
	t.Helper()
	var read struct {
		Links []struct {
			EntityType    string `json:"entity_type"`
			EntityID      string `json:"entity_id"`
			DealRelevance string `json:"deal_relevance"`
		} `json:"links"`
	}
	if status := d.e.Call(t, "GET", "/v1/activities/"+activity, nil, nil, &read); status != http.StatusOK {
		t.Fatalf("reading %s → %d", activity, status)
	}
	for _, link := range read.Links {
		if link.EntityType == "deal" && link.EntityID == deal {
			if link.DealRelevance != relevance {
				t.Errorf("%s's deal link reads %q, want %q", activity, link.DealRelevance, relevance)
			}
			return
		}
	}
	t.Errorf("%s lost its deal link", activity)
}
