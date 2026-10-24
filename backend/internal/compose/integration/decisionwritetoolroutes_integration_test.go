// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The writes that decide something already waiting: decide_duplicate,
// decide_approval, and the import pair preview_import and commit_import. Each
// test asks both doors for the same change on two equal records. It compares
// what each left behind.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

// filedPair makes two contacts that share a phone, so the second file the pair,
// and answers the pair's candidate id.
func filedPair(t *testing.T, d *twoDoors, name, phone string) string {
	t.Helper()
	createdID(t, d.e, "/v1/contacts", AnyMap{
		"full_name": name, "source": "manual",
		"phones": []AnyMap{{"phone": phone, "phone_type": "mobile", "is_primary": true}},
	})
	var made createdWithFollowups
	raw := d.toolSeed(t, "create_record", map[string]any{
		"record_type": "contact",
		"fields": map[string]any{
			"full_name": name + " Brandt",
			"phones":    []map[string]any{{"phone": phone, "phone_type": "work", "is_primary": true}},
		},
	})
	if err := json.Unmarshal(raw, &made); err != nil || len(made.DuplicateCandidates) != 1 {
		t.Fatalf("the second contact filed %d pairs (%v), want one: %s", len(made.DuplicateCandidates), err, raw)
	}
	return made.DuplicateCandidates[0].CandidateID
}

// dispositionOf reads a pair's stored disposition and whether a seat disposed
// it. The route answers the pair and the tool a verdict, so the stored row is
// what the two doors are compared on.
func (d *twoDoors) dispositionOf(t *testing.T, candidate string) (string, bool) {
	t.Helper()
	var disposition string
	var disposed bool
	if err := d.e.Owner.QueryRow(t.Context(),
		`SELECT disposition, disposed_by IS NOT NULL FROM dedupe_candidate WHERE id = $1`, candidate).Scan(&disposition, &disposed); err != nil {
		t.Fatalf("reading pair %s: %v", candidate, err)
	}
	return disposition, disposed
}

// A flagged pair dismissed as not the same, and reopened again, is left alike
// over either door.
func TestDecideDuplicateLeavesTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "dedupe-write-doors", "read", "write")
	viaREST := filedPair(t, d, "Anna Rest", "+4930900001101")
	viaTool := filedPair(t, d, "Anna Tool", "+4930900001102")

	d.restStatus(t, "POST", "/v1/dedupe/candidates/"+viaREST+"/disposition", AnyMap{"disposition": "not_a_duplicate"}, http.StatusOK)
	d.tool(t, "decide_duplicate", map[string]any{"candidate_id": viaTool, "decision": "not_the_same"})
	for _, pair := range []string{viaREST, viaTool} {
		if disposition, disposed := d.dispositionOf(t, pair); disposition != "not_a_duplicate" || !disposed {
			t.Errorf("pair %s is %q (disposed %v) after a dismissal, want not_a_duplicate by a seat", pair, disposition, disposed)
		}
	}
	d.sameEffect(t, "dismiss", viaREST, viaTool)

	d.restStatus(t, "POST", "/v1/dedupe/candidates/"+viaREST+"/undo", AnyMap{}, http.StatusOK)
	d.tool(t, "decide_duplicate", map[string]any{"candidate_id": viaTool, "decision": "reopen"})
	for _, pair := range []string{viaREST, viaTool} {
		if disposition, _ := d.dispositionOf(t, pair); disposition != "open" {
			t.Errorf("pair %s is %q after a reopen, want open", pair, disposition)
		}
	}
	d.sameEffect(t, "reopen", viaREST, viaTool)
}

type decided struct {
	Status         string
	DecisionReason string
	DecidedByKind  string
}

func (d *twoDoors) decisionOn(t *testing.T, approval string) decided {
	t.Helper()
	var out decided
	err := d.e.Owner.QueryRow(t.Context(), `SELECT status, coalesce(decision_reason, ''), CASE WHEN decided_by IS NULL THEN '' ELSE 'seat' END
		FROM approval WHERE id = $1`, approval).Scan(&out.Status, &out.DecisionReason, &out.DecidedByKind)
	if err != nil {
		t.Fatalf("reading approval %s: %v", approval, err)
	}
	return out
}

// stagedApprovalOn answers the one approval a staged call left on a record.
func (d *twoDoors) stagedApprovalOn(t *testing.T, tag string) string {
	t.Helper()
	var id string
	if err := d.e.Owner.QueryRow(t.Context(), `SELECT id::text FROM approval WHERE target_entity_id = $1`, tag).Scan(&id); err != nil {
		t.Fatalf("reading the approval staged on %s: %v", tag, err)
	}
	return id
}

// A staged action approved or rejected by another passport is decided alike
// over either door. The status and the reason are the same, and so are the rows
// behind them.
//
// A site read is the call staged here, as it still waits for a human on a
// colleague's passport.
func TestDecideApprovalLeavesTheSameOnEitherDoor(t *testing.T) {
	proposer := newTwoDoors(t, "decide-write-doors", "read", "write", "enrich")
	decider := proposer.lend(t, "decider", "read", "write")
	stage := func(name string) string {
		domain := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + ".example"
		company := createdID(t, proposer.e, "/v1/companies", AnyMap{
			"source": "manual", "display_name": name,
			"domains": []AnyMap{{"domain": domain, "is_primary": true}},
		})
		proposer.toolRefused(t, "enrich", map[string]any{"company_id": company})
		return proposer.stagedApprovalOn(t, company)
	}
	approveREST, approveTool := stage("Approve via rest"), stage("Approve via tool")
	rejectREST, rejectTool := stage("Reject via rest"), stage("Reject via tool")

	decider.restStatus(t, "POST", "/v1/approvals/"+approveREST+"/approve", AnyMap{"reason": "Checked with the owner"}, http.StatusOK)
	decider.tool(t, "decide_approval", map[string]any{"staged_action_id": approveTool, "decision": "approve", "reason": "Checked with the owner"})
	if rest, tool := decider.decisionOn(t, approveREST), decider.decisionOn(t, approveTool); rest != tool || rest.Status != "approved" || rest.DecisionReason != "Checked with the owner" {
		t.Errorf("an approval left %+v over REST and %+v over the tool, want the same approved decision with its reason", rest, tool)
	}
	decider.sameEffect(t, "approve", approveREST, approveTool, "summary", "diff_hash")

	decider.restStatus(t, "POST", "/v1/approvals/"+rejectREST+"/reject", AnyMap{"reason": "Not this quarter"}, http.StatusOK)
	decider.tool(t, "decide_approval", map[string]any{"staged_action_id": rejectTool, "decision": "reject", "reason": "Not this quarter"})
	if rest, tool := decider.decisionOn(t, rejectREST), decider.decisionOn(t, rejectTool); rest != tool || rest.Status != "rejected" {
		t.Errorf("a rejection left %+v over REST and %+v over the tool, want the same rejected decision", rest, tool)
	}
	decider.sameEffect(t, "reject", rejectREST, rejectTool, "summary", "diff_hash")
}

// sharedRun is the run as the route reads it back. The tool answers a preview
// as {run: {run_id, state}, mapping, columns}. So the preview is compared on
// the run id and state, and the run on what the route stores.
type sharedRun struct {
	Status string `json:"status"`
}

type sharedPreviewReport struct {
	Status      string               `json:"status"`
	RowsRead    int                  `json:"rows_read"`
	Disposition importDispositionDTO `json:"disposition"`
}

// A lead file previewed and then committed is the same run over either door: the
// same report, and the same leads landed.
func TestImportPreviewAndCommitLeaveTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoorsWith(t, "import-write-doors", nil, "read", "write")
	mapping := map[string]string{"Email": "email", "Full Name": "full_name"}
	csv := func(prefix string) string {
		return "Email,Full Name\n" + prefix + "-one@imports.test," + prefix + " One\n" + prefix + "-two@imports.test," + prefix + " Two\n"
	}

	profile, status := uploadCSV(t, d.e, "lead", csv("rest"))
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	restRun := d.restStatus(t, "POST", "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead", "source_ref": profile.SourceRef, "mapping": mapping,
	}, http.StatusAccepted)
	// One agent tests one run at a time, so the route's test ends before the tool's starts.
	importjobtest.Drain(t, d.e)
	toolRun := d.tool(t, "preview_import", map[string]any{"object": "lead", "csv": csv("tool"), "mapping": mapping})
	var previewed struct {
		Run struct {
			RunID string `json:"run_id"`
			State string `json:"state"`
		} `json:"run"`
	}
	if err := json.Unmarshal(toolRun, &previewed); err != nil || previewed.Run.RunID == "" {
		t.Fatalf("the tool's preview names no run (%v): %s", err, toolRun)
	}
	viaREST, viaTool := anIDIn(t, restRun), previewed.Run.RunID
	var viaRESTRun sharedRun
	if err := json.Unmarshal(restRun, &viaRESTRun); err != nil || viaRESTRun.Status != previewed.Run.State {
		t.Errorf("the route's run is %q (%v) and the tool's %q, want the same state", viaRESTRun.Status, err, previewed.Run.State)
	}
	importjobtest.Drain(t, d.e)
	report := func(id string) json.RawMessage {
		return d.restStatus(t, "GET", "/v1/imports/"+id+"/report", nil, http.StatusOK)
	}
	if previewedRows := sameRecords[sharedPreviewReport](t, "preview report", report(viaREST), report(viaTool)); previewedRows.RowsRead != 2 {
		t.Errorf("the preview counts %d rows, want the 2 in the file", previewedRows.RowsRead)
	}
	if leadCount(t, d.e) != 0 {
		t.Errorf("a preview landed %d leads, want none", leadCount(t, d.e))
	}

	d.restStatus(t, "POST", "/v1/imports/"+viaREST+"/approve", nil, http.StatusAccepted)
	d.tool(t, "commit_import", map[string]any{"run_id": viaTool})
	importjobtest.Drain(t, d.e)
	if got := leadCount(t, d.e); got != 4 {
		t.Errorf("the two commits landed %d leads, want the 4 in the two files", got)
	}
	stored := func(id string) json.RawMessage { return d.restStatus(t, "GET", "/v1/imports/"+id, nil, http.StatusOK) }
	committed := sameRecords[sharedRun](t, "commit", stored(viaREST), stored(viaTool))
	if committed.Status == "awaiting_approval" {
		t.Errorf("the committed run is still %q", committed.Status)
	}
}
