// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

func stageSamples(values ...float64) httpserver.Histogram {
	h := httpserver.NewHistogram([]float64{0.1, 1})
	for _, v := range values {
		h.Observe(v)
	}
	return h.Snapshot()
}

func TestTheWorklistStageFamilyRendersOneHistogramPerStage(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	writeWorklistStageMetrics(&out, worklistStageReadings{durations: map[string]httpserver.Histogram{
		"relationship_decay": stageSamples(0.5), "subject_labels": stageSamples(0.02),
	}})
	rendered := out.String()

	for _, want := range []string{
		"# TYPE margince_worklist_stage_duration_seconds histogram",
		`margince_worklist_stage_duration_seconds_bucket{stage="relationship_decay",le="0.1"} 0`,
		`margince_worklist_stage_duration_seconds_bucket{stage="relationship_decay",le="1"} 1`,
		`margince_worklist_stage_duration_seconds_count{stage="subject_labels"} 1`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the scrape does not carry %q:\n%s", want, rendered)
		}
	}
	if strings.Index(rendered, `stage="relationship_decay"`) > strings.Index(rendered, `stage="subject_labels"`) {
		t.Errorf("stages render in name order, so two scrapes read alike:\n%s", rendered)
	}
}

func TestTheWorklistCostFamiliesRenderEachStageAndThePoolWait(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	writeWorklistStageMetrics(&out, worklistStageReadings{
		durations: map[string]httpserver.Histogram{"rank": stageSamples(0.3)},
		costs: map[string]attention.StageCost{
			"rank":                  {Statements: 7, Failed: 1, DB: stageSamples(0.2), App: stageSamples(0.05)},
			attention.OutsideStages: {Statements: 3, DB: stageSamples(0.01), App: stageSamples(0.01)},
		},
		poolWait: stageSamples(0.004),
	})
	rendered := out.String()

	for _, want := range []string{
		"# TYPE margince_worklist_stage_statements_total counter",
		`margince_worklist_stage_statements_total{stage="rank"} 7`,
		`margince_worklist_stage_statements_total{stage="outside_stages"} 3`,
		`margince_worklist_stage_statement_errors_total{stage="rank"} 1`,
		`margince_worklist_stage_statement_errors_total{stage="outside_stages"} 0`,
		"# TYPE margince_worklist_stage_db_seconds histogram",
		`margince_worklist_stage_db_seconds_bucket{stage="rank",le="1"} 1`,
		`margince_worklist_stage_app_seconds_sum{stage="rank"} 0.05`,
		"# TYPE margince_worklist_pool_wait_seconds histogram",
		`margince_worklist_pool_wait_seconds_bucket{le="0.1"} 1`,
		`margince_worklist_pool_wait_seconds_count 1`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the scrape does not carry %q:\n%s", want, rendered)
		}
	}
}

func TestAProcessThatAssembledNoWorklistWritesNoStageFamily(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	writeWorklistStageMetrics(&out, worklistStageReadings{})
	if out.Len() != 0 {
		t.Fatalf("an empty collector rendered %q; no family claims a stage ran", out.String())
	}
}
