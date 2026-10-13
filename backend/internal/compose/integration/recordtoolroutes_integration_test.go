// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The agent tools that answer a question about records, each served on its own
// route by the registry. Every test seeds through the product's writers. It asks
// the route as a passport and the tool over MCP with the same input, and holds
// the two answers equal. An answer stamped with the instant it was read is
// compared without that stamp.

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// One company with a deal, its champion seated on it, and a call the admin
// logged to her.
type accountScene struct {
	company, deal, champion string
}

func newAccountScene(t *testing.T, e *apptest.AppEnv) accountScene {
	t.Helper()
	company, deal := dealAtAnAccount(t, e, "Hollow Ridge", "Ridge renewal")
	s := accountScene{company: company, deal: deal, champion: contactAt(t, e, company, "Ines Champion", "")}
	stakeholder(t, e, deal, s.champion, "champion")
	callAndFold(t, e, s.champion)
	return s
}

// GET /catch-up and GET /meeting-prep take the anchor the tools take, by id or
// by name, and answer the tools' documents byte for byte.
func TestCatchUpAndMeetingPrepRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "anchor-routes", "read")
	scene := newAccountScene(t, d.e)
	for _, anchor := range []map[string]any{
		{"record_type": "company", "record_id": scene.company},
		{"record_type": "contact", "record_name": "Ines Champion", "max_items": 5},
	} {
		query := url.Values{}
		for name, value := range anchor {
			query.Set(name, stringValue(value))
		}
		for path, tool := range map[string]string{"/v1/catch-up": "catch_me_up_on", "/v1/meeting-prep": "prep_for_meeting"} {
			rest := d.rest(t, "GET", path+"?"+query.Encode(), nil)
			if compacted(t, rest) != compacted(t, d.tool(t, tool, anchor)) {
				t.Errorf("GET %s?%s and %s answered different documents:\n%s", path, query.Encode(), tool, rest)
			}
		}
	}

	var picture struct {
		Anchor struct {
			RecordID string `json:"record_id"`
		} `json:"anchor"`
		Sections []json.RawMessage `json:"sections"`
	}
	if err := json.Unmarshal(d.rest(t, "GET", "/v1/catch-up?record_type=contact&record_name=Ines+Champion", nil), &picture); err != nil {
		t.Fatal(err)
	}
	if picture.Anchor.RecordID != scene.champion || len(picture.Sections) == 0 {
		t.Errorf("catching up on the champion by name answered %+v, want her record with its sections", picture)
	}
}

type sharedIntroPaths struct {
	CompanyID string `json:"company_id"`
	Routes    []struct {
		ContactID      string `json:"contact_id"`
		UserID         string `json:"user_id"`
		StrengthBucket string `json:"strength_bucket"`
	} `json:"routes"`
	CandidatesTruncated bool `json:"candidates_truncated"`
}

type sharedAtRisk struct {
	Deals []struct {
		DealID string `json:"deal_id"`
		Risks  []struct {
			Kind       string   `json:"kind"`
			ContactIDs []string `json:"contact_ids"`
		} `json:"risks"`
	} `json:"deals"`
	DealsScanned     int  `json:"deals_scanned"`
	Truncated        bool `json:"truncated"`
	CoverageWithheld bool `json:"coverage_withheld"`
}

// GET /companies/{id}/intro-paths names the colleague who called the champion,
// as intro_path_to does. GET /deals/at-risk flags the deal untouched for two
// months, as at_risk_relationships does.
func TestNetworkRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "network-routes", "read")
	scene := newAccountScene(t, d.e)
	ageLastTouch(t, d.e, scene.deal, 61)

	intro := sameRecords[sharedIntroPaths](t, "intro paths",
		d.rest(t, "GET", "/v1/companies/"+scene.company+"/intro-paths", nil),
		d.tool(t, "intro_path_to", map[string]any{"company_id": scene.company}))
	if len(intro.Routes) != 1 || intro.Routes[0].ContactID != scene.champion {
		t.Errorf("the intro paths are %+v, want one route through the champion", intro.Routes)
	}

	atRisk := sameRecords[sharedAtRisk](t, "at-risk relationships",
		d.rest(t, "GET", "/v1/deals/at-risk", nil),
		d.tool(t, "at_risk_relationships", map[string]any{}))
	if len(atRisk.Deals) != 1 || atRisk.Deals[0].DealID != scene.deal || len(atRisk.Deals[0].Risks) == 0 {
		t.Errorf("the at-risk sweep answered %+v, want the single-threaded deal with its findings", atRisk)
	}
}

type sharedCommitment struct {
	Subject string `json:"subject"`
	State   string `json:"state"`
	TaskID  string `json:"task_id"`
	About   []struct {
		EntityID   string `json:"entity_id"`
		EntityType string `json:"entity_type"`
	} `json:"about"`
}

type sharedCommitments struct {
	Commitments []sharedCommitment `json:"commitments"`
}

type sharedHandoff struct {
	ProjectID       string             `json:"project_id"`
	Name            string             `json:"name"`
	Phase           string             `json:"phase"`
	CompanyID       string             `json:"company_id"`
	OpenCommitments []sharedCommitment `json:"open_commitments"`
	Gaps            []struct {
		Code string `json:"code"`
	} `json:"gaps"`
}

// GET /commitments and GET /projects/{id}/handoff answer what review_commitments
// and prepare_handoff answer. Both stamp the instant they read, so the stamp is
// left out of the comparison.
func TestCommitmentAndHandoffRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "commitment-routes", "read")
	scene := newAccountScene(t, d.e)
	project := createdRecord(t, d.e, "/v1/projects", AnyMap{
		"name": "Ridge rollout", "company_id": scene.company, "source": "manual",
	})
	task := createdRecord(t, d.e, "/v1/tasks", AnyMap{
		"subject": "Send the rollout plan", "due_at": "2026-01-05T09:00:00Z", "source": "manual",
		"links": []AnyMap{{"entity_type": "project", "entity_id": project}},
	})

	commitments := sameRecords[sharedCommitments](t, "commitments",
		d.rest(t, "GET", "/v1/commitments?limit=10", nil),
		d.tool(t, "review_commitments", map[string]any{"limit": 10}))
	if len(commitments.Commitments) != 1 || commitments.Commitments[0].TaskID != task {
		t.Errorf("the open commitments are %+v, want the one overdue task", commitments.Commitments)
	}

	handoff := sameRecords[sharedHandoff](t, "handoff",
		d.rest(t, "GET", "/v1/projects/"+project+"/handoff", nil),
		d.tool(t, "prepare_handoff", map[string]any{"project_id": project}))
	if handoff.ProjectID != project || handoff.CompanyID != scene.company {
		t.Errorf("the handoff is about %s at %s, want the project at the seeded company", handoff.ProjectID, handoff.CompanyID)
	}

	var refused json.RawMessage
	if status := d.e.Call(t, "GET", "/v1/commitments?limit=51", nil, d.bearer, &refused); status != http.StatusUnprocessableEntity {
		t.Errorf("GET /v1/commitments?limit=51 → %d %s, want 422 as the tool refuses it", status, refused)
	}
}

//craft:ignore naked-any an anchor argument is a string or a number, as the tool's schema declares
func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
