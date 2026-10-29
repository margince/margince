// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The worklist's stage families: how long each stage of an assembled day took
// in this process, and what its statements cost. The route histogram says the
// worklist is slow. These say which stage spends the time, in SQL or in Go.

import (
	"io"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

const (
	worklistStageFamily      = "margince_worklist_stage_duration_seconds"
	worklistStatementsFamily = "margince_worklist_stage_statements_total"
	worklistFailedFamily     = "margince_worklist_stage_statement_errors_total"
	worklistDBFamily         = "margince_worklist_stage_db_seconds"
	worklistAppFamily        = "margince_worklist_stage_app_seconds"
	worklistPoolWaitFamily   = "margince_worklist_pool_wait_seconds"
)

// worklistStageReadings is one read of the stage collector.
type worklistStageReadings struct {
	durations map[string]httpserver.Histogram
	costs     map[string]attention.StageCost
	poolWait  httpserver.Histogram
}

func writeWorklistStageMetrics(w io.Writer, r worklistStageReadings) {
	writeStageHistograms(w, worklistStageFamily, "How long each stage of an assembled worklist day took, since process start.", r.durations)
	if len(r.costs) == 0 {
		return
	}
	stages := slices.Sorted(maps.Keys(r.costs))
	writeStageCounter(w, worklistStatementsFamily, "Statements each worklist stage ran, since process start.",
		stages, func(stage string) int64 { return r.costs[stage].Statements })
	writeStageCounter(w, worklistFailedFamily, "Statements each worklist stage ran that ended in an error, since process start.",
		stages, func(stage string) int64 { return r.costs[stage].Failed })
	db, app := map[string]httpserver.Histogram{}, map[string]httpserver.Histogram{}
	for stage, cost := range maps.All(r.costs) {
		db[stage], app[stage] = cost.DB, cost.App
	}
	writeStageHistograms(w, worklistDBFamily, "Database time as the client saw it, per run of each worklist stage.", db)
	writeStageHistograms(w, worklistAppFamily, "Go time per run of each worklist stage, less its statements, pool waits and inner stages.", app)
	httpserver.WriteLine(w, "# HELP %s How long each composed worklist read waited for pool connections.\n", worklistPoolWaitFamily)
	httpserver.WriteLine(w, "# TYPE %s histogram\n", worklistPoolWaitFamily)
	r.poolWait.WriteSeries(w, worklistPoolWaitFamily, "")
}

func writeStageHistograms(w io.Writer, family, help string, byStage map[string]httpserver.Histogram) {
	if len(byStage) == 0 {
		return
	}
	httpserver.WriteLine(w, "# HELP %s %s\n", family, help)
	httpserver.WriteLine(w, "# TYPE %s histogram\n", family)
	for _, stage := range slices.Sorted(maps.Keys(byStage)) {
		byStage[stage].WriteSeries(w, family, "stage="+httpserver.Label(stage))
	}
}

func writeStageCounter(w io.Writer, family, help string, stages []string, count func(stage string) int64) {
	httpserver.WriteLine(w, "# HELP %s %s\n", family, help)
	httpserver.WriteLine(w, "# TYPE %s counter\n", family)
	for _, stage := range stages {
		httpserver.WriteLine(w, "%s{stage=%s} %d\n", family, httpserver.Label(stage), count(stage))
	}
}

func (Server) writeWorklistStageSection(w io.Writer) {
	writeWorklistStageMetrics(w, worklistStageReadings{
		durations: attention.StageTimings(), costs: attention.StageCosts(), poolWait: attention.PoolWaits(),
	})
}
