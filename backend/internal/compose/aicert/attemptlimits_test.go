// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// committedAttemptLimits is the generated file the runtime reads.
var committedAttemptLimits = filepath.Join("..", "..", "modules", "ai", "attemptlimits_gen.go")

func TestTheAttemptLimitTableIsCurrent(t *testing.T) {
	records, err := LoadRecords("records")
	if err != nil {
		t.Fatalf("loading the records: %v", err)
	}
	committed, err := os.ReadFile(committedAttemptLimits)
	if err != nil {
		t.Fatalf("reading %s: %v", committedAttemptLimits, err)
	}
	if err := AttemptLimitDrift(committed, AttemptLimitRows(records)); err != nil {
		t.Fatal(err)
	}
}

func latencyRecord(task ai.Task, model, verdict string, p95ms int64) Record {
	return Record{
		Task: string(task), Provider: "openai_compatible", ServedModel: model, EnvClass: "cloud_frontier",
		Verdict: verdict, LatencyP95: p95ms,
	}
}

func TestTheSlowestRecordThatMayServeSetsTheLimit(t *testing.T) {
	decision := latencyRecord(ai.TaskColdStart, "decider", VerdictCertified, 250000)
	decision.Kind = KindDecision
	local := latencyRecord(ai.TaskColdStart, "on-box", VerdictCertified, 280000)
	local.Provider = "ollama"
	rows := AttemptLimitRows([]Record{
		latencyRecord(ai.TaskColdStart, "fast", VerdictCertified, 10000),
		latencyRecord(ai.TaskColdStart, "degraded", VerdictSupportedDegraded, 70000),
		latencyRecord(ai.TaskColdStart, "refused", VerdictNotSupported, 200000),
		decision, local,
	})
	want := []MeasuredModel{{Provider: "openai_compatible", Model: "degraded"}, {Provider: "openai_compatible", Model: "fast"}}
	if len(rows) != 1 || rows[0].Task != ai.TaskColdStart || rows[0].P95 != 70*time.Second || rows[0].Limit != 105*time.Second ||
		rows[0].Record != "records/cold_start/openai_compatible_degraded_cloud_frontier.json" || !slices.Equal(rows[0].Measured, want) {
		t.Fatalf("rows = %+v, want cold_start held to 105 s by the supported_degraded record, measured on %v", rows, want)
	}
}

// A background task, and a waited task whose latency scales with its input,
// keep the ceiling: neither gets a row however fast its records are.
func TestOnlyAWaitedTaskOfFixedSizeGetsARow(t *testing.T) {
	var records []Record
	for _, task := range ai.AllTasks() {
		if ai.ExecutionModeOf(task) != ai.ExecutionModeInteractive || ai.LatencyScalesWithInput(task) {
			records = append(records, latencyRecord(task, "m", VerdictCertified, 10000))
		}
	}
	if rows := AttemptLimitRows(records); len(records) == 0 || len(rows) != 0 {
		t.Fatalf("%d records of background and input-scaled tasks gave rows %+v, want none", len(records), rows)
	}
}

func TestTheGeneratedLimitIsClampedToTheFloorAndTheCap(t *testing.T) {
	rows := AttemptLimitRows([]Record{
		latencyRecord(ai.TaskRewrite, "m", VerdictCertified, 4303),
		latencyRecord(ai.TaskGrowthFit, "m", VerdictCertified, 223062),
	})
	if len(rows) != 2 || rows[0].Limit != ai.CallCeiling || rows[1].Limit != ai.InteractiveAttemptFloor {
		t.Fatalf("rows = %+v, want growth_fit at the %s cap and rewrite at the %s floor", rows, ai.CallCeiling, ai.InteractiveAttemptFloor)
	}
	table, err := RenderAttemptLimits(rows)
	if err != nil {
		t.Fatalf("RenderAttemptLimits: %v", err)
	}
	for _, want := range []string{`"growth_fit": {limit: 300 * time.Second,`, `"rewrite": {limit: 30 * time.Second,`, `{provider: "openai_compatible", model: "m"},`} {
		if !strings.Contains(string(table), want) {
			t.Errorf("the table does not hold %q:\n%s", want, table)
		}
	}
}

func TestNoRecordRendersAnEmptyTableThatCompiles(t *testing.T) {
	table, err := RenderAttemptLimits(nil)
	if err != nil {
		t.Fatalf("RenderAttemptLimits: %v", err)
	}
	if strings.Contains(string(table), "import") || !strings.Contains(string(table), "var certifiedAttemptTimeouts = map[Task]certifiedLimit{}") {
		t.Fatalf("the empty table is\n%s", table)
	}
}

// The drift names the rows the records give that the table lacks, and the
// committed rows the records no longer give.
func TestTheDriftNamesRowsToAddAndRowsToRemove(t *testing.T) {
	committed, err := RenderAttemptLimits(AttemptLimitRows([]Record{latencyRecord(ai.TaskRewrite, "gone", VerdictCertified, 4000)}))
	if err != nil {
		t.Fatalf("RenderAttemptLimits: %v", err)
	}
	drift := AttemptLimitDrift(committed, AttemptLimitRows([]Record{latencyRecord(ai.TaskNlSearch, "new", VerdictCertified, 4000)}))
	if drift == nil {
		t.Fatal("a table the records no longer give passed")
	}
	for _, want := range []string{`add: "nl_search"`, `add: {provider: "openai_compatible", model: "new"}`, `remove: "rewrite"`, `remove: {provider: "openai_compatible", model: "gone"}`} {
		if !strings.Contains(drift.Error(), want) {
			t.Errorf("the drift does not say %q:\n%s", want, drift)
		}
	}
}

// deadlineSpy is a candidate that notes the deadline each call was sent with.
type deadlineSpy struct {
	*ai.FakeClient
	deadlines []time.Time
}

func (s *deadlineSpy) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	if deadline, ok := ctx.Deadline(); ok {
		s.deadlines = append(s.deadlines, deadline)
	}
	return s.FakeClient.Complete(ctx, req)
}

// The lane measures the latency the limits come from, so it runs a waited task
// at the ceiling. A run cut at the limit would record the limit itself. The
// task and model are the committed row with the lowest limit.
func TestTheLaneMeasuresAWaitedTaskWithoutItsLimit(t *testing.T) {
	records, err := LoadRecords("records")
	if err != nil {
		t.Fatalf("loading the records: %v", err)
	}
	rows := AttemptLimitRows(records)
	if len(rows) == 0 {
		t.Fatal("no committed record gives an interactive limit, so the lane has nothing to lift")
	}
	row := slices.MinFunc(rows, func(a, b AttemptLimitRow) int { return cmp.Compare(a.Limit, b.Limit) })
	site := aitasks.Site{Task: row.Task, Variant: widgetVariant, Kind: ai.SiteKindOneShot}
	scenario := testScenario("basic", wideBands)
	scenario.Task = string(row.Task)
	candidate := &deadlineSpy{FakeClient: ai.NewFakeClient().Script("the widget is blue")}
	// A deadline set before the run, a second short of the ceiling: any try sent
	// at the ceiling ends after it, and one held to the row's limit before it.
	reference, cancel := context.WithTimeout(context.Background(), ai.CallCeiling-time.Second)
	defer cancel()
	referenceDeadline, _ := reference.Deadline()
	_, err = certifyTask(wsContext(t), row.Task, []Scenario{scenario}, censusOfSites(t, site),
		ai.ProviderConfig{Provider: row.Slowest.Provider, Model: row.Slowest.Model}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		ai.ProfileCloudFrontier, 1, quietLogger(),
		&certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithHarnessClient(row.Slowest.Provider, candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(ai.NewFakeClient().Script(scoreJSON(90)))},
		})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	if len(candidate.deadlines) == 0 || !candidate.deadlines[0].After(referenceDeadline) {
		t.Fatalf("%s on %s was sent deadlines %v, want the %s ceiling over its %s limit",
			row.Task, row.Slowest.Model, candidate.deadlines, ai.CallCeiling, row.Limit)
	}
}
