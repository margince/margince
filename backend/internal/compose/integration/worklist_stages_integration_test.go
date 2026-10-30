// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A worklist read times each stage that costs measurable time, the optional
// lanes included, so /metrics can say where a slow day spent its time.
func TestAWorklistReadTimesEachOfItsStages(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	before := attention.StageTimings()

	if status := e.Call(t, "GET", "/v1/worklist?scope=mine&filter=all", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("GET /v1/worklist = %d, want 200", status)
	}
	after := attention.StageTimings()
	for _, stage := range []string{
		"this_morning", "needs_you", "planned", "relationship_decay", "subject_labels",
		"waiting_customers", "plan", "awaiting", "deal_moves",
		"meeting_horizon", "rank", "row_names",
	} {
		key := attention.StageKey{Read: attention.ReadWorklist, Stage: stage}
		if after[key].Count() <= before[key].Count() {
			t.Errorf("stage %s was not timed by the read", stage)
		}
	}
}
