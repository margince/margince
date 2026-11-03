// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// draft_report saves a report's next charts as a draft on the route the
// Analytics screen saves with. Each door drafts on a report of its own, and the
// two reports and the audit rows the drafts left are compared.

import (
	"encoding/json"
	"testing"
)

func TestDraftReportSavesTheSameDraftOnEitherDoor(t *testing.T) {
	d := newTwoDoors(t, "report-draft-doors", "read", "write")
	var catalog struct {
		Metrics []struct {
			ID string `json:"id"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(d.rest(t, "GET", "/v1/analytics/metrics", nil), &catalog); err != nil || len(catalog.Metrics) == 0 {
		t.Fatalf("the catalog names no metric to report on (%v)", err)
	}
	selection := func(period string) AnyMap {
		return AnyMap{"period": period, "target_basis": "month", "close_window": "all_open", "metrics": []string{catalog.Metrics[0].ID}}
	}
	report := func(name string) string {
		return createdRecord(t, d.e, "/v1/analytics/reports", AnyMap{"name": name, "audience": "private", "selection": selection("this_month")})
	}
	viaREST, viaTool := report("Drafted on the route"), report("Drafted by the tool")

	rest := d.restPinned(t, "PUT", "/v1/analytics/reports/"+viaREST+"/draft", AnyMap{"selection": selection("last_month")}, 1)
	tool := d.tool(t, "draft_report", map[string]any{"report_id": viaTool, "selection": selection("last_month")})

	sameStored(t, "a drafted report", rest, tool, viaREST, viaTool, "name")
	d.sameEffect(t, "a report draft", viaREST, viaTool, "name")
	var drafted struct {
		Revision      int64  `json:"revision"`
		DraftRevision *int64 `json:"draft_revision"`
	}
	if err := json.Unmarshal(tool, &drafted); err != nil || drafted.DraftRevision == nil || *drafted.DraftRevision != 2 || drafted.Revision != 1 {
		t.Errorf("the tool's report reads %+v (%v), want revision 1 published with draft 2 waiting", drafted, err)
	}
}
