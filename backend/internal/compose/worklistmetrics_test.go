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

func TestTheWorklistStageFamilyRendersOneHistogramPerReadAndStage(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	writeWorklistStageMetrics(&out, worklistStageReadings{durations: map[attention.StageKey]httpserver.Histogram{
		{Read: attention.ReadWorklist, Stage: "relationship_decay"}: stageSamples(0.5),
		{Read: attention.ReadWorklist, Stage: "subject_labels"}:     stageSamples(0.02),
		{Read: attention.ReadAttention, Stage: "subject_labels"}:    stageSamples(0.03, 0.04),
	}})
	rendered := out.String()

	for _, want := range []string{
		"# TYPE margince_worklist_stage_duration_seconds histogram",
		`margince_worklist_stage_duration_seconds_bucket{read="worklist",stage="relationship_decay",le="0.1"} 0`,
		`margince_worklist_stage_duration_seconds_bucket{read="worklist",stage="relationship_decay",le="1"} 1`,
		`margince_worklist_stage_duration_seconds_count{read="worklist",stage="subject_labels"} 1`,
		`margince_worklist_stage_duration_seconds_count{read="attention",stage="subject_labels"} 2`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the scrape does not carry %q:\n%s", want, rendered)
		}
	}
	if strings.Index(rendered, `read="worklist",stage="relationship_decay"`) > strings.Index(rendered, `read="worklist",stage="subject_labels"`) {
		t.Errorf("stages render in name order, so two scrapes read alike:\n%s", rendered)
	}
}

func TestTheWorklistCostFamiliesRenderEachStageAndThePoolWaitByRead(t *testing.T) {
	t.Parallel()

	rank := attention.StageKey{Read: attention.ReadWorklist, Stage: "rank"}
	outside := attention.StageKey{Read: attention.ReadWorklist, Stage: attention.OutsideStages}
	var out strings.Builder
	writeWorklistStageMetrics(&out, worklistStageReadings{
		durations: map[attention.StageKey]httpserver.Histogram{rank: stageSamples(0.3)},
		costs: map[attention.StageKey]attention.StageCost{
			rank: {
				Statements: 7, Failed: 1, Clipped: 2,
				DB: stageSamples(0.2), PoolWait: stageSamples(0.002), App: stageSamples(0.05),
			},
			outside: {Statements: 3, DB: stageSamples(0.01), PoolWait: stageSamples(0.004), App: stageSamples(0.01)},
		},
		poolWait: map[string]httpserver.Histogram{attention.ReadWorklist: stageSamples(0.006)},
	})
	rendered := out.String()

	for _, want := range []string{
		"# TYPE margince_worklist_stage_statements_total counter",
		`margince_worklist_stage_statements_total{read="worklist",stage="rank"} 7`,
		`margince_worklist_stage_statements_total{read="worklist",stage="outside_stages"} 3`,
		`margince_worklist_stage_statement_errors_total{read="worklist",stage="rank"} 1`,
		`margince_worklist_stage_statement_errors_total{read="worklist",stage="outside_stages"} 0`,
		`margince_worklist_stage_app_clipped_total{read="worklist",stage="rank"} 2`,
		"# TYPE margince_worklist_stage_db_seconds histogram",
		`margince_worklist_stage_db_seconds_bucket{read="worklist",stage="rank",le="1"} 1`,
		`margince_worklist_stage_pool_wait_seconds_sum{read="worklist",stage="outside_stages"} 0.004`,
		`margince_worklist_stage_app_seconds_sum{read="worklist",stage="rank"} 0.05`,
		"# TYPE margince_worklist_pool_wait_seconds histogram",
		`margince_worklist_pool_wait_seconds_bucket{read="worklist",le="0.1"} 1`,
		`margince_worklist_pool_wait_seconds_count{read="worklist"} 1`,
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
