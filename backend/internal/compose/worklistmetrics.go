// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The worklist's stage families: how long each stage of a composed read took
// in this process, and what its pgx calls cost. The route histogram says the
// worklist is slow. These say which stage of which read spends the time.

import (
	"cmp"
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
	worklistClippedFamily    = "margince_worklist_stage_app_clipped_total"
	worklistDBFamily         = "margince_worklist_stage_db_seconds"
	worklistStageWaitFamily  = "margince_worklist_stage_pool_wait_seconds"
	worklistAppFamily        = "margince_worklist_stage_app_seconds"
	worklistPoolWaitFamily   = "margince_worklist_pool_wait_seconds"
)

// worklistStageReadings is one read of the stage collector.
type worklistStageReadings struct {
	durations map[attention.StageKey]httpserver.Histogram
	costs     map[attention.StageKey]attention.StageCost
	poolWait  map[string]httpserver.Histogram
}

func writeWorklistStageMetrics(w io.Writer, r worklistStageReadings) {
	writeStageHistograms(w, worklistStageFamily, "How long each stage of a composed worklist read took, since process start.", r.durations)
	if len(r.costs) == 0 {
		return
	}
	counters := []struct {
		family, help string
		count        func(attention.StageCost) int64
	}{
		{
			worklistStatementsFamily, "pgx statements each stage ran, since process start.",
			func(c attention.StageCost) int64 { return c.Statements },
		},
		{
			worklistFailedFamily, "pgx statements each stage ran that ended in an error, since process start.",
			func(c attention.StageCost) int64 { return c.Failed },
		},
		{
			worklistClippedFamily, "Stage runs whose pgx calls and inner stages took longer than the stage itself.",
			func(c attention.StageCost) int64 { return c.Clipped },
		},
	}
	for _, counter := range counters {
		httpserver.WriteLine(w, "# HELP %s %s\n", counter.family, counter.help)
		httpserver.WriteLine(w, "# TYPE %s counter\n", counter.family)
		for _, key := range sortedStageKeys(r.costs) {
			httpserver.WriteLine(w, "%s{%s} %d\n", counter.family, stageLabels(key), counter.count(r.costs[key]))
		}
	}
	db, wait, app := map[attention.StageKey]httpserver.Histogram{}, map[attention.StageKey]httpserver.Histogram{}, map[attention.StageKey]httpserver.Histogram{}
	for key, cost := range maps.All(r.costs) {
		db[key], wait[key], app[key] = cost.DB, cost.PoolWait, cost.App
	}
	writeStageHistograms(w, worklistDBFamily, "Time inside pgx statement calls, per run of each stage.", db)
	writeStageHistograms(w, worklistStageWaitFamily, "Time inside pool Acquire calls, per run of each stage.", wait)
	writeStageHistograms(w, worklistAppFamily, "Rest of each stage run: its time less pgx calls, pool waits and inner stages.", app)
	httpserver.WriteLine(w, "# HELP %s Time inside pool Acquire calls, per composed read.\n", worklistPoolWaitFamily)
	httpserver.WriteLine(w, "# TYPE %s histogram\n", worklistPoolWaitFamily)
	for _, read := range slices.Sorted(maps.Keys(r.poolWait)) {
		r.poolWait[read].WriteSeries(w, worklistPoolWaitFamily, "read="+httpserver.Label(read))
	}
}

func sortedStageKeys[V any](byKey map[attention.StageKey]V) []attention.StageKey {
	return slices.SortedFunc(maps.Keys(byKey), func(a, b attention.StageKey) int {
		return cmp.Or(cmp.Compare(a.Read, b.Read), cmp.Compare(a.Stage, b.Stage))
	})
}

func stageLabels(key attention.StageKey) string {
	return "read=" + httpserver.Label(key.Read) + ",stage=" + httpserver.Label(key.Stage)
}

func writeStageHistograms(w io.Writer, family, help string, byKey map[attention.StageKey]httpserver.Histogram) {
	if len(byKey) == 0 {
		return
	}
	httpserver.WriteLine(w, "# HELP %s %s\n", family, help)
	httpserver.WriteLine(w, "# TYPE %s histogram\n", family)
	for _, key := range sortedStageKeys(byKey) {
		byKey[key].WriteSeries(w, family, stageLabels(key))
	}
}

func (Server) writeWorklistStageSection(w io.Writer) {
	writeWorklistStageMetrics(w, worklistStageReadings{
		durations: attention.StageTimings(), costs: attention.StageCosts(), poolWait: attention.PoolWaits(),
	})
}
