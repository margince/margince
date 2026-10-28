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

// A worklist read times every stage it runs, the optional lanes included,
// so /metrics can say where a slow day spent its time.
func TestAWorklistReadTimesEachOfItsStages(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	before := attention.StageTimings()

	if status := e.Call(t, "GET", "/v1/worklist?scope=mine&filter=all", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("GET /v1/worklist = %d, want 200", status)
	}
	after := attention.StageTimings()
	for _, stage := range []string{
		"end_of_day", "this_morning", "needs_you", "planned", "relationship_decay",
		"done_for_you", "subject_labels", "price_day", "waiting_customers", "pins_and_done",
		"plan", "awaiting", "deal_moves", "meeting_horizon", "rank", "row_names",
	} {
		if after[stage].Count() <= before[stage].Count() {
			t.Errorf("stage %s was not timed by the read", stage)
		}
	}
}
