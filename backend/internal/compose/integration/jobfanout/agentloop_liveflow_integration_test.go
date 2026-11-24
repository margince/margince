// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/agentloope2e/live"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
)

// liveCaseDeadline bounds one run through Tick. A model that stalls costs this
// much and no more, and the run then fails its checks like any other.
const liveCaseDeadline = 15 * time.Minute

// liveServe puts the model under test behind the agent and answers the service
// that runs it. A scripted model queues its steps and answers the scripted
// service. The paid lane answers the service over the production route.
type liveServe func(t *testing.T, re *runnerEnv, run loopRun) *compose.RunnerService

// liveLane is the code path of a live pass. The scripted proof of it and the
// paid pass differ only in setup and serve, so what the proof shows holds for
// the paid pass.
type liveLane struct {
	cfg      live.Config
	traceDir string
	// setup builds one fresh environment per run, with the recorder of its model
	// route, which is nil when the model is scripted.
	setup func(t *testing.T) (*runnerEnv, *live.Recorder)
	serve liveServe
}

// certify runs the scenario cfg.Runs times, each in a subtest of its own, and
// files the verdict. A failed subtest is a harness failure, and it ends the pass
// before another run is paid for.
func (l liveLane) certify(t *testing.T, sc agentloope2e.Scenario) (agentloope2e.Verdict, error) {
	t.Helper()
	return live.Certify(context.Background(), l.cfg, sc, func(run int) (live.Observed, error) {
		var seen live.Observed
		if !t.Run(fmt.Sprintf("run%d", run), func(t *testing.T) { seen = l.attempt(t, sc, run) }) {
			return live.Observed{}, errors.New("the harness failed, see above")
		}
		return seen, nil
	})
}

// attempt seeds a fresh workspace and holds the seed to what the tools serve. It
// runs the agent through the worker's own Tick path and reads the end state back.
func (l liveLane) attempt(t *testing.T, sc agentloope2e.Scenario, run int) live.Observed {
	t.Helper()
	re, recorder := l.setup(t)
	world := re.seedLoopWorld(t, sc)
	re.requireDayHeadroom(t, sc)
	probe := re.probeSeed(t, sc, world)
	svc := l.serve(t, re, loopRun{sc: sc, world: world, probe: probe})

	trigger := fmt.Sprintf("%s:e2e-live-%d-%d", sc.Agent, run, wallNow().UnixNano())
	re.enqueue(t, string(sc.Agent), trigger, &re.passportID)
	re.tickWithin(t, svc, liveCaseDeadline)
	re.assertDayHeld(t, sc, world)

	observed := re.snapshotOf(t, world, trigger)
	for _, write := range observed.snap.Writes {
		if write.Tool == unrecordedCall {
			t.Fatalf("harness: audit rows on %q belong to no executed tool call", write.Subject)
		}
	}
	status, steps, _ := re.runRow(t, trigger)
	trace := live.Trace{
		Scenario: sc.Name, Binding: l.cfg.Binding, Run: run, Status: status, Steps: steps,
		Failures: agentloope2e.Check(sc, observed.snap), Writes: observed.snap.Writes,
	}
	if recorder != nil {
		trace.Calls = recorder.Calls()
		if runner.Outcome(status) != runner.OutcomeCompleted && !recorder.Answered() {
			t.Fatalf("harness: the run ended %s and the model route answered no call; check the key and the model name", status)
		}
	}
	if l.traceDir != "" {
		path, err := trace.Write(l.traceDir)
		if err != nil {
			t.Fatalf("harness: writing the trace: %v", err)
		}
		t.Logf("trace: %s", path)
	}
	return live.Observed{Snapshot: observed.snap, SeededAt: world.seededAt}
}

// tickWithin is tick over a given service and under a deadline.
func (re *runnerEnv) tickWithin(t *testing.T, svc *compose.RunnerService, limit time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(re.wsCtx, limit)
	defer cancel()
	if err := svc.Tick(ctx, wallNow()); err != nil {
		t.Fatal(err)
	}
}
