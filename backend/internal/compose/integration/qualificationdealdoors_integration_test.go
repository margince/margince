// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestQualificationAddsStakeholdersToTheSameExistingDealOnBothDoors(t *testing.T) {
	d := newTwoDoors(t, "qualification-existing-deal", "read", "write")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Renewal buyer", "source": "manual"})
	deal := createdID(t, d.e, "/v1/deals", AnyMap{"name": "Annual renewal", "company_id": company, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual"})
	createLead := func(email string) string {
		id := createdID(t, d.e, "/v1/leads", AnyMap{"full_name": "Renewal buyer", "company_name": "Renewal buyer", "email": email, "source": "manual"})
		d.restStatusWith(t, "POST", "/v1/leads/"+id+"/link-company", AnyMap{"company_id": company}, nil, http.StatusOK)
		return id
	}
	rest, tool := createLead("rest-buyer@renewal.test"), createLead("tool-buyer@renewal.test")
	d.restPinned(t, "POST", "/v1/leads/"+rest+"/promote", AnyMap{"trigger": "human_qualify", "existing_deal_id": deal}, versionOf(t, d.storedLead(t, rest)))
	d.tool(t, "promote_lead", map[string]any{"lead_id": tool, "trigger": "human_qualify", "existing_deal_id": deal})
	for _, id := range []string{rest, tool} {
		var lead struct {
			Status          string `json:"status"`
			QualifiedDealID string `json:"qualified_deal_id"`
		}
		if err := json.Unmarshal(d.storedLead(t, id), &lead); err != nil {
			t.Fatal(err)
		}
		if lead.Status != "promoted" || lead.QualifiedDealID != deal {
			t.Fatalf("qualification result: %+v", lead)
		}
	}
	var edges struct {
		Data []struct {
			ContactID string `json:"contact_id"`
			DealID    string `json:"deal_id"`
		}
	}
	if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/relationships?kind=deal_stakeholder&deal_id="+deal, nil, http.StatusOK), &edges); err != nil {
		t.Fatal(err)
	}
	if len(edges.Data) != 2 || edges.Data[0].ContactID == edges.Data[1].ContactID {
		t.Fatalf("stakeholder committee: %+v", edges.Data)
	}
	for _, edge := range edges.Data {
		if edge.DealID != deal {
			t.Fatalf("wrong deal: %+v", edge)
		}
	}
}

func TestQualificationRejectsConflictingDealChoicesOnBothDoors(t *testing.T) {
	d := newTwoDoors(t, "qualification-conflicting-deal", "read", "write")
	id := createdID(t, d.e, "/v1/leads", AnyMap{"full_name": "Buyer", "email": "buyer@choices.test", "source": "manual"})
	deal := "00000000-0000-4000-8000-000000000001"
	refusal := d.restStatus(t, "POST", "/v1/leads/"+id+"/promote", AnyMap{"trigger": "human_qualify", "existing_deal_id": deal, "deal": AnyMap{}}, http.StatusUnprocessableEntity)
	for _, field := range []string{"deal", "existing_deal_id"} {
		if !strings.Contains(string(refusal), `"field":"`+field+`"`) {
			t.Fatalf("REST refusal omitted %s: %s", field, refusal)
		}
	}
	refused := d.mcp.CallRefused(t, "promote_lead", map[string]any{"lead_id": id, "trigger": "human_qualify", "existing_deal_id": deal, "deal": map[string]any{}})
	for _, field := range []string{"deal", "existing_deal_id"} {
		if !strings.Contains(refused, field) {
			t.Fatalf("MCP refusal omitted %s: %s", field, refused)
		}
	}
	var lead sharedLead
	if err := json.Unmarshal(d.storedLead(t, id), &lead); err != nil {
		t.Fatal(err)
	}
	if lead.Status == "promoted" {
		t.Fatal("conflicting choices promoted the lead")
	}
}

func TestQualificationCanOpenADealAtTheChosenEmployerOnBothDoors(t *testing.T) {
	d := newTwoDoors(t, "qualification-new-employer-deal", "read", "write")
	for _, door := range []string{"rest", "tool"} {
		t.Run(door, func(t *testing.T) {
			id := createdID(t, d.e, "/v1/leads", AnyMap{"full_name": "Independent buyer " + door, "company_name": "Independent Works " + door, "email": door + "-qualification@gmail.com", "source": "manual"})
			choice := AnyMap{"trigger": "human_qualify", "create_company": true, "deal": AnyMap{"name": "Agreed scope " + door}}
			if door == "rest" {
				d.restPinned(t, "POST", "/v1/leads/"+id+"/promote", choice, versionOf(t, d.storedLead(t, id)))
			} else {
				choice["lead_id"] = id
				d.tool(t, "promote_lead", choice)
			}
			var lead struct {
				QualifiedDealID string `json:"qualified_deal_id"`
			}
			if err := json.Unmarshal(d.storedLead(t, id), &lead); err != nil {
				t.Fatal(err)
			}
			if lead.QualifiedDealID == "" {
				t.Fatal("qualification returned no deal")
			}
			var deal struct {
				CompanyID string `json:"company_id"`
				Name      string `json:"name"`
			}
			if err := json.Unmarshal(d.restStatus(t, "GET", "/v1/deals/"+lead.QualifiedDealID, nil, http.StatusOK), &deal); err != nil {
				t.Fatal(err)
			}
			if deal.CompanyID == "" || deal.Name != "Agreed scope "+door {
				t.Fatalf("qualified deal lost the requested employer or scope: %+v", deal)
			}
		})
	}
}
