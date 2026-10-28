// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

func TestTheWorklistStageFamilyRendersOneHistogramPerStage(t *testing.T) {
	t.Parallel()

	decay := httpserver.NewHistogram([]float64{0.1, 1})
	decay.Observe(0.5)
	labels := httpserver.NewHistogram([]float64{0.1, 1})
	labels.Observe(0.02)
	var out strings.Builder
	writeWorklistStageMetrics(&out, map[string]httpserver.Histogram{
		"relationship_decay": decay.Snapshot(), "subject_labels": labels.Snapshot(),
	})
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

func TestAProcessThatAssembledNoWorklistWritesNoStageFamily(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	writeWorklistStageMetrics(&out, nil)
	if out.Len() != 0 {
		t.Fatalf("an empty collector rendered %q; no family claims a stage ran", out.String())
	}
}
