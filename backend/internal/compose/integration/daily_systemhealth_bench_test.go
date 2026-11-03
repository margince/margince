// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var dailySystemHealthFlow = dailyFlow{
	Name: "system_health", ID: "PERF-11", Budget: Perf11Budget,
	Calls: func(dailyInput) []dailyCall { return []dailyCall{{Name: "overview", Path: "/v1/admin/system-health"}} },
}

var dailySystemHealthDiagnosticsFlow = dailyFlow{
	Name: "system_health_diagnostics", ID: "PERF-12", Budget: Perf12Budget, PerCall: true,
	Calls: func(dailyInput) []dailyCall {
		return []dailyCall{
			{Name: "history", Path: "/v1/admin/job-health/history"},
			{Name: "capture", Path: "/v1/admin/capture-health"},
			{Name: "automatic", Path: "/v1/admin/automatic-work-health"},
		}
	},
}

func seedDailySystemHealth(t *testing.T, e *apptest.AppEnv, scale float64, owner Seat) {
	t.Helper()
	count := max(100, int(10_000*scale))
	enqueueHealthFixtures(t, e, count)
	if _, err := e.Owner.Exec(t.Context(), `UPDATE river_job SET
 state=CASE WHEN id%3=0 THEN 'discarded'::river_job_state WHEN id%3=1 THEN 'cancelled'::river_job_state ELSE state END,
 finalized_at=CASE WHEN id%3 IN (0,1) THEN now() ELSE finalized_at END
 WHERE kind=$1`, (compose.CaptureClassifyArgs{}).Kind()); err != nil {
		t.Fatal(err)
	}
	var actual int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind=$1`, (compose.CaptureClassifyArgs{}).Kind()).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != count {
		t.Fatalf("health fixture census %d, want %d", actual, count)
	}
	ws := apptest.InstallationWorkspaceUUID(t.Context(), t, e.Pool)
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), ws), "system:health_benchmark")
	captured := max(10, int(1000*scale))
	if err := compose.SeedSystemHealthFixture(ctx, e.Pool, ids.MustParse(owner.UserID), owner.Email, captured, clocktest.Now(t)); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Sources []struct {
			ID            string `json:"id"`
			AutomaticWork *struct {
				Created int `json:"created"`
			} `json:"automatic_work"`
		}
	}
	mustCall(t, e, "GET", "/v1/admin/system-health", nil, 200, &report)
	for _, source := range report.Sources {
		if source.ID == "automatic" && (source.AutomaticWork == nil || source.AutomaticWork.Created == 0) {
			t.Fatal("automatic-work fixture is empty")
		}
	}
	t.Logf("System health domain fixtures: %d captured messages, %d refused records, one recorded restore drill", captured, captured)
	t.Logf("System health fixture: %d queued, discarded and cancelled runs through the production inserter", count)
}

func TestSystemHealthBudgets(t *testing.T) {
	scale, err := dailyScale()
	if err != nil {
		t.Fatal(err)
	}
	embedder := embedReindexRouter(t, "health-bench-embed-v1")
	e := apptest.SetupAppWithOptions(t, compose.WithEmbedReindex(embedder, insertOnlyRunner(t)))
	seedRouterBinding(t, e, embedder)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	seats := createDailySeats(t, e)
	seedDailyCorpus(t, e, seats, scale)
	made := seedDailyApprovals(t, e, seats, scale)
	made = applyDailyApprovals(t, e, made)
	seedDailySystemHealth(t, e, scale, seats.Reps[0])
	admin := Seat{Name: "admin", Role: "admin", Client: e.Client}
	rows := runDailyFlow(t, e, dailySystemHealthFlow, dailyInput{Seat: admin})
	rows = append(rows, runDailyFlow(t, e, dailySystemHealthDiagnosticsFlow, dailyInput{Seat: admin})...)
	rows = append(rows, measureConcurrentHealth(t, e, admin))
	fixtureNote := healthBenchmarkFixtureNote(t, e)
	measurements := make([]BudgetMeasurement, 0, len(rows))
	results := make([]DailyResult, 0, len(rows))
	for _, row := range rows {
		if scale != 1 {
			row.Measurement.Caveat = fmt.Sprintf("development scale %g", scale)
		}
		row.Measurement.Note = joinNotes(row.Measurement.Note, fixtureNote)
		measurements = append(measurements, row.Measurement)
		results = append(results, row.Result)
	}
	record := newPerfRecord("bench-system-health", benchPostgresVersion(e.Owner), measurements)
	record.Corpus = dailyCorpusFacts(scale, seats)
	record.Corpus.PendingApprovals = made.Pending
	if err := DailyGate(results, scale); err != nil {
		t.Fatal(err)
	}
	writePerfRecordTo(t, dailyRecordDir(t, scale), record)
}

func measureConcurrentHealth(t *testing.T, e *apptest.AppEnv, seat Seat) dailyRow {
	t.Helper()
	calls := make([]dailyCall, 8)
	for i := range calls {
		calls[i] = dailyCall{Name: "overview", Path: "/v1/admin/system-health"}
	}
	var tally dailyTally
	round := func(counted bool) ([]time.Duration, error) {
		start := time.Now()
		answers, err := fetchRound(seat, e.TS.URL, calls, true)
		elapsed := time.Since(start)
		if err != nil {
			return nil, err
		}
		for _, answer := range answers {
			if err := dailySystemHealthFlow.admit(answer, tallied(&tally, counted)); err != nil {
				return nil, err
			}
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			if _, err := writer.Write(answer.body); err != nil {
				return nil, err
			}
			if err := writer.Close(); err != nil {
				return nil, err
			}
			if compressed.Len() > 50*1024 {
				return nil, fmt.Errorf("summary exceeds 50 KiB compressed: %d", compressed.Len())
			}
		}
		return []time.Duration{elapsed}, nil
	}
	samples, err := dailySampling(1, round)
	if err != nil {
		t.Fatal(err)
	}
	spec := dailyRowSpec{flow: dailySystemHealthFlow.Name, id: "PERF-11", name: "system_health_eight_readers", budget: Perf11Budget, gated: true, note: "Eight simultaneous overview requests; whole-round latency; each response <= 50 KiB gzip"}
	return dailyMeasuredRow(t, seat, spec, samples[0], tally)
}

func healthBenchmarkFixtureNote(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var summary crmcontracts.SystemHealth
	var jobs crmcontracts.JobHealth
	var capture crmcontracts.CaptureHealth
	mustCall(t, e, "GET", "/v1/admin/system-health", nil, 200, &summary)
	mustCall(t, e, "GET", "/v1/admin/job-health", nil, 200, &jobs)
	mustCall(t, e, "GET", "/v1/admin/capture-health", nil, 200, &capture)
	runs, refused, automatic := 0, 0, 0
	restored := false
	for _, kind := range jobs.Kinds {
		if kind.Kind == (compose.CaptureClassifyArgs{}).Kind() {
			runs += kind.Waiting + kind.Running + kind.Retrying + kind.Dead
		}
	}
	for _, source := range summary.Sources {
		if source.Id == "intake" && source.Count != nil {
			refused = *source.Count
		}
		if source.Id == "automatic" && source.AutomaticWork != nil {
			automatic = source.AutomaticWork.Created
		}
		if source.Id == "recovery" {
			restored = source.Code == "recovery_verified"
		}
	}
	classified := capture.Classifier.Pending + capture.Classifier.Unsure + capture.Classifier.Exhausted
	if runs == 0 || refused == 0 || automatic == 0 || classified == 0 || !restored {
		t.Fatalf("empty health fixture: runs=%d refused=%d automatic=%d classified=%d restored=%t", runs, refused, automatic, classified, restored)
	}
	return fmt.Sprintf("Retained job runs: %d; sender classifications: %d; refused inputs: %d; automatic items: %d; recorded passing restore drill. HTTP reads only; browser rendering is measured separately; Go schedulers: %d.", runs, classified, refused, automatic, runtime.GOMAXPROCS(0))
}
