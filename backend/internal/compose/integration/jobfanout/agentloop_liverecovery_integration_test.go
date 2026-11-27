// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The fixture owns the first refused call. The candidate owns every later choice.
type seededRecoveryBrain struct {
	runner.Brain
	reply    string
	sent     atomic.Bool
	observed atomic.Bool
}

func (b *seededRecoveryBrain) Complete(ctx context.Context, req model.Request) (model.Response, runner.Meta, error) {
	if !b.sent.Swap(true) {
		return model.Response{Text: b.reply}, runner.Meta{ModelID: "fixture/recovery", Tier: "fixture"}, nil
	}
	if !b.observed.Swap(true) {
		observation := req.System
		for _, message := range req.Messages {
			observation += message.Content
		}
		if !strings.Contains(observation, "tool call refused:") {
			return model.Response{}, runner.Meta{}, fmt.Errorf("candidate did not receive the seeded refusal")
		}
	}
	return b.Brain.Complete(ctx, req)
}

func (re *runnerEnv) withSeededRecovery(t *testing.T, served *compose.RunnerService, run loopRun) *compose.RunnerService {
	t.Helper()
	target := run.probe.deals[run.sc.Worklist[0]]
	args := integration.AnyMap{"record_type": "deal", "id": target}
	switch run.sc.Recovery {
	case "bad_arguments":
		args["id"] = "not-a-record-id"
	case "unknown_field":
		args["obsolete_field"] = true
	case "field_shape":
		args["id"] = []string{target}
	}
	path := re.scriptedPath
	if served == re.live {
		if re.livePath == nil {
			t.Fatal("live service has no model path")
		}
		path = *re.livePath
	}
	path.AgentLoop = &seededRecoveryBrain{Brain: path.AgentLoop, reply: toolStep(t, "read_record", args)}
	var options []compose.RunnerOption
	if run.sc.Recovery == "unsupported" {
		var failed atomic.Bool
		options = append(options, compose.WithRunnerReadFailureForTest(func(_ context.Context, ref datasource.EntityRef) error {
			if ref.Type == datasource.EntityDeal && ref.ID.String() == target && !failed.Swap(true) {
				return fmt.Errorf("record reader: %w", apperrors.ErrUnsupportedBySoR)
			}
			return nil
		}))
	}
	return re.newService(path, options...)
}

func measureLiveRecovery(sc agentloope2e.Scenario, snap agentloope2e.Snapshot, steps []runner.Step) *aitasks.LoopRecoveryMeasure {
	measure := &aitasks.LoopRecoveryMeasure{}
	if len(steps) < 2 || steps[0].ModelID != "fixture/recovery" {
		return measure
	}
	refused := func(step runner.Step) bool {
		return step.Admission == runner.AdmissionRefused || step.Admission == "rejected"
	}
	if !refused(steps[0]) {
		return measure
	}
	first := steps[1]
	measure.FirstStepCorrect = first.Tool == "read_brief" && !refused(first)
	if sc.Agent == agentloope2e.OvernightSweep {
		measure.FirstStepCorrect = first.Tool == "whats_slipping_this_week" && !refused(first)
	}
	for i, step := range steps {
		if refused(step) {
			measure.Refusals++
		}
		for _, previous := range steps[:i] {
			if refused(previous) && step.Tool == previous.Tool && (previous.TerminalRefusal || sameRecoveryArgs(step.Args, previous.Args)) {
				measure.TerminalRepeated = true
			}
		}
	}
	sc.Recovery = ""
	measure.Completed = len(agentloope2e.Check(sc, snap)) == 0
	measure.Recovered = measure.Completed && !measure.TerminalRepeated
	return measure
}

func TestLiveLaneRecoversEachRealRefusalAndFilesTheDatabaseResult(t *testing.T) {
	for _, kind := range []string{"bad_arguments", "unknown_field", "field_shape", "unsupported"} {
		t.Run(kind, func(t *testing.T) {
			lane, sc, _, _ := scriptedLane(t, "morning_brief_recovers_"+kind, 1)
			candidate := scriptedModelNamed(t, "does_the_job")
			original := candidate.script
			candidate.script = func(t *testing.T, run loopRun) []string {
				steps := original(t, run)
				if run.sc.Recovery != "unsupported" {
					return steps
				}

				// The candidate can still read the brief, its activity evidence and catch-up.
				// A terminal read refusal must not be repeated even when the goal finishes.
				var recovered []string
				for _, step := range steps {
					if !strings.Contains(step, `"read_record"`) {
						recovered = append(recovered, step)
					}
				}
				return recovered
			}
			lane.serve = servedBy(candidate)
			verdict, err := lane.certify(t, sc)
			if err != nil {
				t.Fatal(err)
			}
			if len(verdict.Runs) != 1 || !verdict.Runs[0].ChecksPassed() {
				t.Fatalf("recovery did not reach the real write goal: %+v", verdict.Runs)
			}
			measure := verdict.Runs[0].Recovery
			if measure == nil || !measure.FirstStepCorrect || !measure.Completed || !measure.Recovered || measure.Refusals < 1 || measure.TerminalRepeated {
				t.Fatalf("missing worker recovery proof: %+v", measure)
			}
		})
	}
}

func TestLiveRecoveryCannotPassARepeatedTerminalCallOrAnUnfinishedGoal(t *testing.T) {
	sc := agentloope2e.Scenario{Agent: agentloope2e.MorningBrief, Recovery: "unsupported", NoWrites: true}
	seed := runner.Step{ModelID: "fixture/recovery", Tool: "read_record", Args: []byte(`{"id":"same","record_type":"deal"}`), Admission: runner.AdmissionRefused, TerminalRefusal: true}
	repeated := seed
	repeated.ModelID = "candidate"
	repeated.Args = []byte(`{"record_type":"deal","id":"another"}`)
	snap := agentloope2e.Snapshot{RunStatus: runner.OutcomeCompleted}
	measure := measureLiveRecovery(sc, snap, []runner.Step{seed, {Tool: "read_brief", Admission: "executed"}, repeated})
	if !measure.TerminalRepeated || measure.Recovered {
		t.Fatalf("repeated terminal refusal passed: %+v", measure)
	}
	snap.RunStatus = runner.OutcomeDegraded
	measure = measureLiveRecovery(sc, snap, []runner.Step{seed, {Tool: "read_brief", Admission: "executed"}})
	if measure.Completed || measure.Recovered {
		t.Fatal("unfinished worker was called recovered")
	}
}

func sameRecoveryArgs(a, b []byte) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return bytes.Equal(a, b)
	}
	return reflect.DeepEqual(left, right)
}

func TestACompletedWorkerStillFailsWhenItRepeatsARefusedRequest(t *testing.T) {
	lane, sc, _, _ := scriptedLane(t, "morning_brief_recovers_bad_arguments", 1)
	candidate := scriptedModelNamed(t, "repeats_refused_request")
	lane.serve = servedBy(candidate)
	verdict, err := lane.certify(t, sc)
	if err != nil {
		t.Fatal(err)
	}
	if len(verdict.Runs) != 1 {
		t.Fatalf("got %d runs", len(verdict.Runs))
	}
	got := verdict.Runs[0]
	if got.Recovery == nil || !got.Recovery.Completed || !got.Recovery.TerminalRepeated || got.ChecksPassed() {
		t.Fatalf("repeated permanent-input refusal escaped the real worker check: %+v", got)
	}
}
