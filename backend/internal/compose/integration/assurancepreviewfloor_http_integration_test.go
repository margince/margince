// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// A check's preview counts what a pass would raise over the whole workspace.
// With the reader and one colleague, each count less the reader's own is the
// colleague's. While the switch is off the preview is refused.
func TestIndividualPerformanceOffRefusesAPreviewOfOneColleaguesChecks(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithAssuranceRun(nil))
	me, colleague, _ := twoSellerTeam(t, e, "preview-floor")
	stages := apptest.DiscoverSeededPipeline(t, e)
	soon := clocktest.Now(t).AddDate(0, 0, 10)
	for _, owner := range []string{me, colleague} {
		deal := createdRecord(t, e, "/v1/deals", AnyMap{
			"name": "Overdue deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
			"owner_id": owner, "expected_close_date": soon.Format(time.DateOnly),
		})
		// The close date passes without anyone moving it, as the check finds it.
		if _, err := OwnerConn(t).Exec(context.Background(), `UPDATE deal SET expected_close_date = $1 WHERE id = $2`,
			soon.AddDate(0, 0, -40), deal); err != nil {
			t.Fatalf("letting the close date pass: %v", err)
		}
	}
	const preview = "/v1/forecast/assurance/preview"
	var counted struct {
		Findings []struct {
			Count int `json:"count"`
		} `json:"findings"`
	}
	if status := e.Call(t, http.MethodGet, preview, nil, nil, &counted); status != http.StatusOK || len(counted.Findings) == 0 {
		t.Fatalf("with the switch on, the preview = %d %+v, want 200 with the overdue deals counted", status, counted)
	}
	mustCall(t, e, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, http.StatusOK, nil)
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, http.MethodGet, preview, nil, nil, &problem); status != http.StatusForbidden || problem.Code != "individual_performance_off" {
		t.Errorf("with the switch off, the preview = %d %q, want 403 individual_performance_off", status, problem.Code)
	}
}
