// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// decide_approval_bundle decides one act's proposals together. The route and
// the tool must bring every member to the same outcome and status.

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

// stagedPerBundle is how many proposals each staged act holds.
const stagedPerBundle = 2

type bundleMember struct{ Kind, Outcome, Status string }

// bundleOutcomes reads what each member of a decided bundle came to. The
// route nests each approval under `data` and the tool lists the members flat,
// and the members' ids differ per bundle, so the answers are compared on the
// kind, outcome and status each member came to.
func bundleOutcomes(t *testing.T, raw json.RawMessage) []bundleMember {
	t.Helper()
	var body struct {
		Data []struct {
			Approval struct {
				Kind   string `json:"kind"`
				Status string `json:"status"`
			} `json:"approval"`
			Outcome string `json:"outcome"`
		} `json:"data"`
		Members []struct {
			Kind    string `json:"kind"`
			Status  string `json:"status"`
			Outcome string `json:"outcome"`
		} `json:"members"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("a bundle decision does not decode: %v\n%s", err, raw)
	}
	var out []bundleMember
	for _, m := range body.Data {
		out = append(out, bundleMember{m.Approval.Kind, m.Outcome, m.Approval.Status})
	}
	for _, m := range body.Members {
		out = append(out, bundleMember{m.Kind, m.Outcome, m.Status})
	}
	if len(out) != stagedPerBundle {
		t.Fatalf("a bundle decision answered %d members, want the %d staged: %s", len(out), stagedPerBundle, raw)
	}
	return out
}

// One act's proposals approved or rejected together are decided alike over
// either door: the same outcome and status for every member.
func TestDecideApprovalBundleLeavesTheSameOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "bundle-write-doors", "read", "write")
	company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Bundle Doors", "source": "manual"})
	approveREST, approveTool := stageBundleRows(t, d.e, company, stagedPerBundle), stageBundleRows(t, d.e, company, stagedPerBundle)
	rejectREST, rejectTool := stageBundleRows(t, d.e, company, stagedPerBundle), stageBundleRows(t, d.e, company, stagedPerBundle)

	restApproved := d.restStatus(t, "POST", "/v1/approval-bundles/"+approveREST.String()+"/approve", AnyMap{"reason": "Read looks right"}, http.StatusOK)
	toolApproved := d.tool(t, "decide_approval_bundle", map[string]any{"bundle_id": approveTool.String(), "decision": "approve", "reason": "Read looks right"})
	rest, tool := bundleOutcomes(t, restApproved), bundleOutcomes(t, toolApproved)
	if !slices.Equal(rest, tool) || rest[0].Status != "approved" {
		t.Errorf("an approved bundle came to %+v over REST and %+v over the tool, want the same approved members", rest, tool)
	}

	restRejected := d.restStatus(t, "POST", "/v1/approval-bundles/"+rejectREST.String()+"/reject", AnyMap{"reason": "Wrong company"}, http.StatusOK)
	toolRejected := d.tool(t, "decide_approval_bundle", map[string]any{"bundle_id": rejectTool.String(), "decision": "reject", "reason": "Wrong company"})
	rest, tool = bundleOutcomes(t, restRejected), bundleOutcomes(t, toolRejected)
	if !slices.Equal(rest, tool) || rest[0].Status != "rejected" {
		t.Errorf("a rejected bundle came to %+v over REST and %+v over the tool, want the same rejected members", rest, tool)
	}
}
