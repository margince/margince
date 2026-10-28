// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The worklist's stage family: how long each stage of an assembled day took in
// this process. The route histogram says the worklist is slow; this says which
// lane or stage it spends that time in.

import (
	"io"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

const worklistStageFamily = "margince_worklist_stage_duration_seconds"

func writeWorklistStageMetrics(w io.Writer, stages map[string]httpserver.Histogram) {
	if len(stages) == 0 {
		return
	}
	httpserver.WriteLine(w, "# HELP %s How long each stage of an assembled worklist day took, since process start.\n", worklistStageFamily)
	httpserver.WriteLine(w, "# TYPE %s histogram\n", worklistStageFamily)
	for _, stage := range slices.Sorted(maps.Keys(stages)) {
		stages[stage].WriteSeries(w, worklistStageFamily, "stage="+httpserver.Label(stage))
	}
}

func (Server) writeWorklistStageSection(w io.Writer) {
	writeWorklistStageMetrics(w, attention.StageTimings())
}
