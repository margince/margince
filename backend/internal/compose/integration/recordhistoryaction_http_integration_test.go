// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The `action` filter at the DOOR, which is where the refusal lives.
//
// The store's half — a known verb this record never saw answers an honest empty
// page — is asserted against ListRecordHistory, which takes a filter rather than
// a request. A verb the installation does not record at all never reaches it:
// the handler refuses that, so it has to be asserted where the handler is.
//
// The two answers must stay different. An empty page is a fact about the
// record; a 422 is a mistyped request. A caller who reads the first for the
// second concludes something false about their data (issue #1611).

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestRecordHistoryRefusesAVerbThisInstallationDoesNotRecord(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pid := seedPersonWithActivity(t, e)

	var problem struct {
		Code    string `json:"code"`
		Details struct {
			Errors []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"errors"`
		} `json:"details"`
	}
	// `promoted` is the plausible typo for `promote` — the shape a caller
	// actually sends, rather than a string nothing could be mistaken for.
	status := e.Call(t, "GET", "/v1/records/person/"+pid+"/history?action=promoted", nil, nil, &problem)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 — an unknown verb answered as an empty page tells a "+
			"caller this record never saw something, which is not what they asked", status)
	}
	if len(problem.Details.Errors) == 0 || problem.Details.Errors[0].Field != "action" {
		t.Errorf("the refusal must name the parameter that was wrong, got %+v", problem)
	}
	if len(problem.Details.Errors) > 0 && problem.Details.Errors[0].Code != "unknown_action" {
		t.Errorf("code = %q, want unknown_action", problem.Details.Errors[0].Code)
	}

	// And a verb the installation DOES record is admitted, so the refusal is
	// about the vocabulary rather than about the parameter existing at all.
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/records/person/"+pid+"/history?action=create", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("a recorded verb = %d, want 200", status)
	}
	if len(page.Data) == 0 {
		t.Error("filtering by a verb this record DOES carry answered nothing")
	}
}
