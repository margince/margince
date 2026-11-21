// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	rows := AttemptLimitRows([]Record{
		latencyRecord(ai.TaskColdStart, "fast", VerdictCertified, 10000),
		latencyRecord(ai.TaskColdStart, "degraded", VerdictSupportedDegraded, 70000),
		latencyRecord(ai.TaskColdStart, "refused", VerdictNotSupported, 200000),
		decision,
	})
	if len(rows) != 1 || rows[0].Task != ai.TaskColdStart || rows[0].P95 != 70*time.Second || rows[0].Limit != 105*time.Second ||
		rows[0].Record != "records/cold_start/openai_compatible_degraded_cloud_frontier.json" {
		t.Fatalf("rows = %+v, want cold_start held to 105 s by the supported_degraded record", rows)
	}
}

func TestOnlyAWaitedTaskOfFixedSizeGetsARow(t *testing.T) {
	rows := AttemptLimitRows([]Record{
		latencyRecord(ai.TaskAccountScan, "m", VerdictCertified, 10000),
		latencyRecord(ai.TaskTranscript, "m", VerdictCertified, 10000),
	})
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none: a background task and a transcript keep the ceiling", rows)
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
	for _, want := range []string{`"growth_fit": 300 * time.Second,`, `"rewrite": 30 * time.Second,`} {
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
	if strings.Contains(string(table), "import") || !strings.Contains(string(table), "var certifiedAttemptTimeouts = map[Task]time.Duration{}") {
		t.Fatalf("the empty table is\n%s", table)
	}
}

// deadlineSpy is a candidate that notes the deadline each call was sent with.
type deadlineSpy struct {
	*ai.FakeClient
	deadlines []time.Duration
}

func (s *deadlineSpy) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	if deadline, ok := ctx.Deadline(); ok {
		s.deadlines = append(s.deadlines, time.Until(deadline))
	}
	return s.FakeClient.Complete(ctx, req)
}

// The lane measures the latency the limits come from, so it runs a waited task
// at the ceiling: a run cut at the limit would record the limit itself.
func TestTheLaneMeasuresAWaitedTaskWithoutItsLimit(t *testing.T) {
	candidate := &deadlineSpy{FakeClient: ai.NewFakeClient().Script("the widget is blue")}
	judgeFake := ai.NewFakeClient().Script(scoreJSON(90))
	_, err := certifyTask(wsContext(t), ai.TaskSummarize, []Scenario{testScenario("basic", wideBands)}, testCensus(t),
		ai.ProviderConfig{Provider: "gemini", Model: "gemini-3.1-flash-lite-preview"}, ai.ProviderConfig{Provider: ai.ProviderFake, Model: "judge"},
		ai.ProfileEUHosted, 1, quietLogger(),
		&certifyHooks{
			candidateOpts: []ai.LocalOption{ai.WithHarnessClient("gemini", candidate)},
			judgeOpts:     []ai.LocalOption{ai.WithFakeClient(judgeFake)},
		})
	if err != nil {
		t.Fatalf("certifyTask: %v", err)
	}
	limit := ai.TaskDefaults(ai.TaskSummarize).AttemptTimeout
	if len(candidate.deadlines) == 0 || candidate.deadlines[0] <= limit {
		t.Fatalf("the candidate was sent deadlines %v, want more than summarize's %s limit", candidate.deadlines, limit)
	}
}
