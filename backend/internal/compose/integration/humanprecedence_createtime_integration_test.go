// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Automatic work edits a record a human created in one call, with no approval.
// The audit row is what a human reads to see the change and Undo it.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAutomaticWorkOverwritesCreateTimeHumanValuesAndNothingWaits(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var pipelines struct {
		Data []struct {
			ID     string `json:"id"`
			Stages []struct {
				ID       string `json:"id"`
				Semantic string `json:"semantic"`
			} `json:"stages"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &pipelines); status != http.StatusOK || len(pipelines.Data) == 0 {
		t.Fatalf("pipelines → %d", status)
	}
	pipelineID := pipelines.Data[0].ID
	var stageID string
	for _, s := range pipelines.Data[0].Stages {
		if s.Semantic == "open" {
			stageID = s.ID
			break
		}
	}

	// The human creates the deal exactly as the shipped form does: every
	// field in ONE post.
	var deal struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": "Acme renewal", "pipeline_id": pipelineID, "stage_id": stageID,
		"amount_minor": 25000000, "currency": "EUR",
		"expected_close_date": "2026-12-31", "source": "manual",
	}, nil, &deal); status != http.StatusCreated {
		t.Fatalf("human deal create → %d", status)
	}

	automaticPatch(t, e, "deal", deal.ID, AnyMap{
		"amount_minor": 100, "expected_close_date": "2027-06-30",
	})

	var current struct {
		AmountMinor       int64  `json:"amount_minor"`
		ExpectedCloseDate string `json:"expected_close_date"`
	}
	if status := e.Call(t, "GET", "/v1/deals/"+deal.ID, nil, nil, &current); status != http.StatusOK {
		t.Fatalf("read back → %d", status)
	}
	if current.AmountMinor != 100 {
		t.Errorf("amount_minor = %d, want the automatic edit's 100", current.AmountMinor)
	}
	if current.ExpectedCloseDate != "2027-06-30" {
		t.Errorf("expected_close_date = %q, want the automatic edit's 2027-06-30", current.ExpectedCloseDate)
	}
	if n := countApprovals(t, e); n != 0 {
		t.Errorf("%d approvals staged, want none", n)
	}
}

// An agent fills an empty field with no approval.
func TestAgentFillsAnEmptyFieldWithoutApproval(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	contactID, agentToken := stageContactAndAgent(t, e, "Petra Human", "empty-field agent")
	bearer := map[string]string{"Authorization": "Bearer " + agentToken}

	var patched struct {
		Title          string `json:"title"`
		StagedApproval *struct {
			ApprovalID string `json:"approval_id"`
		} `json:"staged_approval"`
	}
	if status := e.Call(t, "PATCH", "/v1/contacts/"+contactID, AnyMap{"title": "Founder"}, bearer, &patched); status != http.StatusOK {
		t.Fatalf("agent fills an empty field → %d", status)
	}
	if patched.StagedApproval != nil && patched.StagedApproval.ApprovalID != "" {
		t.Errorf("filling an empty title staged approval %s",
			patched.StagedApproval.ApprovalID)
	}
	if patched.Title != "Founder" {
		t.Errorf("title = %q, want the agent's fill to have landed", patched.Title)
	}
}

// automaticPatch sends update_record as automatic work and fails the test on any
// refusal: the whole patch applies.
func automaticPatch(t *testing.T, e *apptest.AppEnv, recordType, id string, fields AnyMap) {
	t.Helper()
	call, err := json.Marshal(AnyMap{"record_type": recordType, "id": id, "fields": fields})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := automaticInvoker(t, e)("update_record", string(call)); err != nil {
		t.Fatalf("automatic %s patch → %v", recordType, err)
	}
}
