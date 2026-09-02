// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The meeting brief over HTTP.
//
// Every other test of this surface calls the service directly, which proves the
// assembly and proves nothing about the route: the generated stub answers 501,
// and a handler set that stopped being embedded would still compile. A 200
// carrying the contract envelope is what says a client is actually served.
//
// It also pins the plan's own shape on the wire, which the service tests cannot
// see — they read Go structs, and a field that never marshalled would look
// present to every one of them.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestTheMeetingBriefServesItsPlanOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Brief E2E", "brief@fable.test", "Admin")

	var person struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/people",
		AnyMap{"full_name": "Ana Roth"}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create person → %d", status)
	}
	// Logged through the product's own writer rather than inserted: a fixture
	// the real writer never produces proves nothing about what the real writer
	// hands the brief.
	var meeting struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Retrofit review",
		"links": []AnyMap{{"entity_type": "person", "entity_id": person.ID}},
	}, nil, &meeting); status != http.StatusCreated {
		t.Fatalf("log meeting → %d", status)
	}

	// Pointers throughout, so an absent key, a null and an empty array are
	// three distinguishable answers. The contract requires each of these, and a
	// renderer that read a missing array as "none" would say the record holds
	// nothing when the field simply never marshalled.
	var body struct {
		ActivityID  *string            `json:"activity_id"`
		GeneratedBy *string            `json:"generated_by"`
		Sections    *[]json.RawMessage `json:"sections"`
		Plan        *struct {
			GeneratedBy *string `json:"generated_by"`
			Readiness   *string `json:"readiness"`
			MeetingType *struct {
				Value      *string `json:"value"`
				Confidence *string `json:"confidence"`
			} `json:"meeting_type"`
			AccountArc *[]json.RawMessage `json:"account_arc"`
			Questions  *[]json.RawMessage `json:"questions"`
			Unknowns   *[]json.RawMessage `json:"unknowns"`
			Advance    *struct {
				Minimum  *json.RawMessage `json:"minimum"`
				Best     *json.RawMessage `json:"best"`
				Fallback *json.RawMessage `json:"fallback"`
			} `json:"advance"`
		} `json:"plan"`
	}
	status := e.Call(t, "GET", "/v1/activities/"+meeting.ID+"/meeting-brief", nil, nil, &body)
	if status != http.StatusOK {
		t.Fatalf("GET meeting-brief → %d, want 200 "+
			"(501 means the generated stub answered and the handler set is no longer embedded)", status)
	}
	if body.Plan == nil {
		t.Fatal("the brief arrived with no plan")
	}
	if body.Plan.Readiness == nil || *body.Plan.Readiness == "" {
		t.Error("the plan does not say how ready it is, so a client cannot decide whether to lead with it")
	}
	if body.Plan.MeetingType == nil || body.Plan.MeetingType.Value == nil {
		t.Fatal("the plan does not say what kind of meeting this is")
	}
	if body.Plan.Unknowns == nil {
		t.Error("`unknowns` is absent rather than an empty array; a client cannot tell 'no gaps' from 'not answered'")
	}
	if body.Plan.AccountArc == nil {
		t.Error("`account_arc` is absent rather than an empty array")
	}
	// The advance is required in full: a meeting with two ways to close and no
	// third is not a plan a rep can act on.
	if body.Plan.Advance == nil || body.Plan.Advance.Minimum == nil ||
		body.Plan.Advance.Best == nil || body.Plan.Advance.Fallback == nil {
		t.Errorf("the advance is incomplete: %+v", body.Plan.Advance)
	}
	// A record with nothing in it still gets a plan, and the plan says so
	// rather than inventing preparation.
	if body.Plan.GeneratedBy == nil || *body.Plan.GeneratedBy == "" {
		t.Error("the plan does not name its writer")
	}
}
