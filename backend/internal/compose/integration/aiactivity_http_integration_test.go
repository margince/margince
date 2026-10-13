// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The handler reaches the route only because an embedded handler set shadows
// the generated 501 stub, and that shadowing is asserted at COMPILE time
// alone: a handler set that stopped being embedded would still build if the
// stub answered in its place. The stub answers 501, so a 200 carrying the
// contract envelope is the proof that the real handler serves this route.
//
// The second half is the operation's `x-agent-access: human-only`
// declaration. A personal read of what the AI did on your behalf is
// exactly the surface an injected agent would use to learn what it is
// permitted to do unobserved, so the refusal is asserted through a real
// minted passport rather than trusted to the generated policy table.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestMyAiActivityServesTheRealHandlerToAHuman(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// Pointers, so an absent key and a null are both distinguishable from a
	// present empty array — the contract requires all three fields, and the
	// panel renders "at rest" from an empty `running`, not from a missing one.
	// Decoding into slices also fails the call outright if either field
	// arrives as anything but a JSON array.
	var body struct {
		AsOf      *string            `json:"as_of"`
		Running   *[]json.RawMessage `json:"running"`
		Recent    *[]json.RawMessage `json:"recent"`
		LiveTotal *int               `json:"live_total"`
	}
	status := e.Call(t, "GET", "/v1/me/ai-activity", nil, nil, &body)
	if status != http.StatusOK {
		t.Fatalf("human GET /v1/me/ai-activity → %d, want 200 "+
			"(501 means the generated stub answered and the handler set is no longer embedded)", status)
	}
	if body.AsOf == nil || *body.AsOf == "" {
		t.Errorf("as_of is absent or empty: the reader cannot tell how fresh this answer is")
	}
	if body.Running == nil {
		t.Errorf("running is absent: the contract requires it, and an empty array is how the rail says the AI is at rest")
	}
	if body.Recent == nil {
		t.Errorf("recent is absent: the contract requires it even on a day with no settled occurrence")
	}
	if body.LiveTotal == nil || *body.LiveTotal != 0 {
		t.Errorf("live_total = %v, want a present 0: an absent total is unknown, not an AI at rest", body.LiveTotal)
	}
}

func TestMyAiActivityRefusesAnAgentBearer(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "agent activity read probe", "scopes": []string{"read"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}

	// 403 exactly, and the sentinel code with it: a 404 would be a pass for
	// the wrong reason, since the point is that the refusal lands before the
	// handler ever looks for a run.
	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, "GET", "/v1/me/ai-activity", nil,
		map[string]string{"Authorization": "Bearer " + minted.Token}, &problem)
	if status != http.StatusForbidden {
		t.Errorf("agent GET /v1/me/ai-activity → %d, want 403 (the contract declares it human-only)", status)
	}
	if problem.Code != "permission_denied" {
		t.Errorf("agent GET /v1/me/ai-activity → code %q, want permission_denied", problem.Code)
	}
}

// Which kinds queries the real router refuses. `?kinds=` binds to one empty
// member, not to a zero-length slice, and the enum refusal covers it like any
// other value outside the contract's vocabulary.
//
// The accepted rows assert only that a legal query is not refused. A fresh
// workspace has no AI activity, so narrowing is proved against seeded
// occurrences in TestTheBoundFallsInsideTheKindsTheCallerAskedFor.
func TestTheKindsQueryIsRefusedOrAcceptedAsTheBinderDeliversIt(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	for _, tc := range []struct {
		name  string
		query string
		want  int
		code  string
	}{
		{"a list that went missing", "?kinds=", http.StatusUnprocessableEntity, "invalid"},
		{"a hand-typed name", "?kinds=summarise", http.StatusUnprocessableEntity, "invalid"},
		{"one bad name beside a good one", "?kinds=morning_brief&kinds=summarise", http.StatusUnprocessableEntity, "invalid"},
		{"the kinds a client draws", "?kinds=morning_brief&kinds=document_extract", http.StatusOK, ""},
		{"every kind the contract carries", "?kinds=morning_brief&kinds=summarize&kinds=voice_build", http.StatusOK, ""},
		{"no filter at all", "", http.StatusOK, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The per-field breakdown lives under `details.errors`, which is the
			// contract's shape and NOT a top-level `fields` — decoding the wrong
			// key yields an empty list that reads exactly like a refusal that
			// named no field.
			var body struct {
				Running *[]json.RawMessage `json:"running"`
				Recent  *[]json.RawMessage `json:"recent"`
				Details struct {
					Errors []struct {
						Field string `json:"field"`
						Code  string `json:"code"`
					} `json:"errors"`
				} `json:"details"`
			}
			status := e.Call(t, "GET", "/v1/me/ai-activity"+tc.query, nil, nil, &body)
			if status != tc.want {
				t.Fatalf("GET %s → %d, want %d", tc.query, status, tc.want)
			}
			if tc.code == "" {
				// Both arrays present, per the contract — an accepted query has
				// to come back as the envelope a client can iterate, not merely
				// as a 200.
				if body.Running == nil || body.Recent == nil {
					t.Errorf("a served feed is missing an array: running=%v recent=%v", body.Running, body.Recent)
				}
				return
			}
			// The CODE is the half a client acts on, and it is the half that was
			// wrong: both refusals are 422, so a status assertion alone would
			// have passed over the defect this case exists for.
			got := body.Details.Errors
			if len(got) != 1 || got[0].Field != "kinds" || got[0].Code != tc.code {
				t.Errorf("details.errors = %+v, want one kinds/%s", got, tc.code)
			}
		})
	}
}
