// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Read tools over workflow state: approvals, the brief, import runs and lists.
// Each test makes the state through the product's writers, then asks the route
// as a passport and the tool over MCP with the same input.

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A staged approval lists and reads the same on GET /approvals and
// list_approvals, and on GET /approvals/{id} and read_approval.
func TestApprovalRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "approval-read-doors", "read", "write")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	deal := openDeal(t, d.e, stages, "Approval door deal")
	win := AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}
	if status := d.e.Call(t, "POST", "/v1/deals/"+deal+"/progress", win, d.bearer, nil); status != http.StatusForbidden {
		t.Fatalf("a won move should stage an approval → %d", status)
	}
	staged := stagedProgressions(t, d.e, deal)
	if len(staged) != 1 {
		t.Fatalf("the won move staged %v, want one approval", staged)
	}

	pending := d.rest(t, "GET", "/v1/approvals?status=pending", nil)
	pendingTool := d.tool(t, "list_approvals", map[string]any{"status": "pending"})
	bothDoorsName(t, "pending approvals", pending, pendingTool, staged[0], deal)
	one := d.rest(t, "GET", "/v1/approvals/"+staged[0], nil)
	oneTool := d.tool(t, "read_approval", map[string]any{"staged_action_id": staged[0]})
	bothDoorsName(t, "one approval", one, oneTool, staged[0], deal)

	// The ids are labelled differently on the two doors; the kind and the
	// status the approval is in must be the same.
	want := sharedApproval{Kind: "progress_deal", Status: "pending"}
	if got := sameRecords[sharedApproval](t, "one approval", one, oneTool); got != want {
		t.Errorf("the approval reads %+v, want %+v", got, want)
	}
	for door, answer := range map[string]json.RawMessage{"route": pending, "tool": pendingTool} {
		var listed struct {
			Data      []sharedApproval `json:"data"`
			Approvals []sharedApproval `json:"approvals"`
		}
		if err := json.Unmarshal(answer, &listed); err != nil {
			t.Fatalf("the %s's approvals do not decode: %v\n%s", door, err, answer)
		}
		if rows := slices.Concat(listed.Data, listed.Approvals); len(rows) != 1 || rows[0] != want {
			t.Errorf("the %s lists %+v, want the one pending %+v", door, rows, want)
		}
	}
}

type sharedApproval struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

// A run of a CSV import reads the same on GET /imports/{id} and read_import_run,
// and its report on GET /imports/{id}/report and read_import_report.
func TestImportRoutesAnswerAsTheirTools(t *testing.T) {
	d := newTwoDoors(t, "import-read-doors", "read")
	run, _ := importContactFile(t, d.e, "Email,Full Name,Title\nada@doors.example,Ada Doors,Engineer\n")

	bothDoorsName(t, "import run",
		d.rest(t, "GET", "/v1/imports/"+run, nil), d.tool(t, "read_import_run", map[string]any{"run_id": run}), run)
	// The route also carries links; the run, its status and its counts must agree.
	report := sameRecords[sharedImportReport](t, "import report",
		d.rest(t, "GET", "/v1/imports/"+run+"/report", nil),
		memberOf(t, d.tool(t, "read_import_report", map[string]any{"run_id": run}), "report"))
	if report.RunID != run || report.RowsRead != 1 {
		t.Errorf("the import report reads %+v, want run %s with its 1 row", report, run)
	}
}

type sharedImportReport struct {
	RunID       string               `json:"run_id"`
	Status      string               `json:"status"`
	RowsRead    int                  `json:"rows_read"`
	Disposition importDispositionDTO `json:"disposition"`
}

type listCreated struct {
	ID string `json:"id"`
}

// A list finds, reads, shows its members, its history and why a member belongs
// the same on its routes and on read_lists. Its preview mode has no passport
// route: POST /filters/preview is human-only, so only the tool can run it.
func TestListRoutesAnswerAsReadLists(t *testing.T) {
	d := newTwoDoorsWith(t, "list-read-doors", []compose.Option{compose.WithListsEnabled(true)}, "read")
	var list listCreated
	if status := d.e.Call(t, "POST", "/v1/lists", AnyMap{
		"name": "Door shortlist", "entity_type": "contact", "list_type": "static",
	}, nil, &list); status != http.StatusCreated {
		t.Fatalf("creating the list → %d", status)
	}
	member := createdRecord(t, d.e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Lisbeth Listed"})
	if status := d.e.Call(t, "POST", "/v1/lists/"+list.ID+"/members",
		AnyMap{"entity_type": "contact", "entity_id": member}, nil, nil); status >= http.StatusBadRequest {
		t.Fatalf("adding a member → %d", status)
	}

	bothDoorsName(t, "find lists",
		d.rest(t, "GET", "/v1/lists", nil),
		d.tool(t, "read_lists", map[string]any{"mode": "find"}), list.ID)
	bothDoorsName(t, "get a list",
		d.rest(t, "GET", "/v1/lists/"+list.ID, nil),
		d.tool(t, "read_lists", map[string]any{"mode": "get", "list_id": list.ID}), list.ID)
	bothDoorsName(t, "list members",
		d.rest(t, "GET", "/v1/lists/"+list.ID+"/members", nil),
		d.tool(t, "read_lists", map[string]any{"mode": "members", "list_id": list.ID}), member)
	bothDoorsName(t, "list history",
		d.rest(t, "GET", "/v1/lists/"+list.ID+"/history", nil),
		d.tool(t, "read_lists", map[string]any{"mode": "history", "list_id": list.ID}), member)
	bothDoorsName(t, "why a member",
		d.rest(t, "GET", "/v1/lists/"+list.ID+"/members/"+member+"/why", nil),
		d.tool(t, "read_lists", map[string]any{"mode": "why", "list_id": list.ID, "record_id": member}), member)
}

// The morning brief reads the same on GET /brief and read_brief.
func TestBriefRouteAnswersAsTheTool(t *testing.T) {
	d := newTwoDoors(t, "brief-read-doors", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	createDealClosingThisWeek(t, d.e, stages, "Closing this week")
	createDealClosingThisWeek(t, d.e, stages, "Also closing this week")
	if status := d.e.Call(t, "POST", "/v1/brief", nil, nil, nil); status != http.StatusCreated {
		t.Fatalf("generating the brief → %d", status)
	}

	var rest, tool struct {
		LocalDay string `json:"local_day"`
		Items    []struct {
			DealID string `json:"deal_id"`
		} `json:"items"`
	}
	restRaw, toolRaw := d.rest(t, "GET", "/v1/brief", nil), d.tool(t, "read_brief", map[string]any{})
	if err := json.Unmarshal(restRaw, &rest); err != nil {
		t.Fatalf("the route's brief does not decode: %v\n%s", err, restRaw)
	}
	if err := json.Unmarshal(toolRaw, &tool); err != nil {
		t.Fatalf("the tool's brief does not decode: %v\n%s", err, toolRaw)
	}
	if rest.LocalDay == "" || rest.LocalDay != tool.LocalDay {
		t.Errorf("the route reads the brief for %q, the tool for %q", rest.LocalDay, tool.LocalDay)
	}
	if len(rest.Items) != 2 || len(tool.Items) != 2 {
		t.Fatalf("the route lists %d brief items, the tool %d, want the 2 seeded deals on both", len(rest.Items), len(tool.Items))
	}
	for i := range rest.Items {
		if rest.Items[i].DealID != tool.Items[i].DealID {
			t.Errorf("brief item %d: the route names deal %s, the tool %s", i, rest.Items[i].DealID, tool.Items[i].DealID)
		}
	}
}
