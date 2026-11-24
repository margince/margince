// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package live

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/compose/agentloope2e"
)

// Observed is the end state of one run, as the harness read it back from the
// database. SeededAt is the instant the seed's offsets counted from.
type Observed struct {
	Snapshot agentloope2e.Snapshot
	SeededAt time.Time
}

// Config is one pass over one scenario.
type Config struct {
	// Root is the repository root and VerdictDir the folder verdicts are filed in.
	Root, VerdictDir string
	Binding          agentloope2e.Binding
	Judge            agentloope2e.Judge
	// JudgeModel is the model Judge asks, as CommandJudge.Model reports it. Empty
	// means the pinned judge.
	JudgeModel string
	// Runs is how many times the scenario is run; the pass bar reads them all.
	Runs int
	// Now stamps the verdict; time.Now when nil.
	Now func() time.Time
}

// Certify runs a scenario Runs times through attempt and files one verdict. It
// stops at the first harness failure and files nothing. A harness failure is an
// error from attempt or from the judge. A verdict from fewer runs than asked
// for would claim a measurement nobody made.
//
// A run that fails a deterministic check is not judged, since the pass bar
// counts it out whatever the judge says.
func Certify(ctx context.Context, cfg Config, sc agentloope2e.Scenario, attempt func(run int) (Observed, error)) (agentloope2e.Verdict, error) {
	if cfg.Runs < 1 {
		return agentloope2e.Verdict{}, errors.New("live: a verdict needs at least one run")
	}
	stamp, err := CurrentStamp(cfg.Root, cfg.JudgeModel, sc, cfg.Binding)
	if err != nil {
		return agentloope2e.Verdict{}, err
	}
	var runs []agentloope2e.Run
	for i := 1; i <= cfg.Runs; i++ {
		seen, err := attempt(i)
		if err != nil {
			return agentloope2e.Verdict{}, fmt.Errorf("live: %s run %d of %d: %w", sc.Name, i, cfg.Runs, err)
		}
		run := agentloope2e.Run{Failures: agentloope2e.Check(sc, seen.Snapshot)}
		if run.ChecksPassed() {
			if run.Criteria, err = cfg.Judge.Grade(ctx, sc.Criteria, agentloope2e.Answer(sc, seen.SeededAt, seen.Snapshot.Writes)); err != nil {
				return agentloope2e.Verdict{}, fmt.Errorf("live: %s run %d of %d: %w", sc.Name, i, cfg.Runs, err)
			}
		}
		runs = append(runs, run)
	}
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	verdict := agentloope2e.Verdict{
		Scenario: sc.Name, Agent: sc.Agent, Binding: cfg.Binding, Judge: stamp.Judge,
		SelfJudged: agentloope2e.SameFamily(cfg.Binding.Model, stamp.Judge),
		Runs:       runs, Outcome: agentloope2e.Decide(runs), Stamp: stamp,
		RanAt: now().UTC().Format(time.RFC3339),
	}
	return verdict, verdict.Write(cfg.VerdictDir)
}

// Line is one verdict and whether it still describes the product.
type Line struct {
	Verdict agentloope2e.Verdict
	// Moved names the stamp parts that no longer match, empty when current.
	Moved []string
}

// Current says whether the verdict is current.
func (l Line) Current() bool { return len(l.Moved) == 0 }

// Lines reads every verdict under dir and compares each with the stamp this tree
// would give it, judged by the named model. A verdict for a scenario no longer
// in the tree names it as moved.
func Lines(root, dir, judgeModel string) ([]Line, error) {
	verdicts, err := agentloope2e.ReadVerdicts(dir)
	if err != nil {
		return nil, err
	}
	scenarios, err := agentloope2e.LoadScenarios(filepath.Join(root, ScenarioDir))
	if err != nil {
		return nil, err
	}
	var lines []Line
	for _, verdict := range verdicts {
		i := slices.IndexFunc(scenarios, func(sc agentloope2e.Scenario) bool { return sc.Name == verdict.Scenario })
		if i < 0 {
			lines = append(lines, Line{Verdict: verdict, Moved: []string{"scenario removed"}})
			continue
		}
		current, err := CurrentStamp(root, judgeModel, scenarios[i], verdict.Binding)
		if err != nil {
			return nil, err
		}
		lines = append(lines, Line{Verdict: verdict, Moved: verdict.Stamp.Moved(current)})
	}
	return lines, nil
}

// IsCurrent says whether a verdict for this scenario and binding is filed under
// dir and still describes the product. None filed is not current.
func IsCurrent(root, dir, judgeModel string, sc agentloope2e.Scenario, binding agentloope2e.Binding) (bool, error) {
	probe := agentloope2e.Verdict{Scenario: sc.Name, Binding: binding}
	if _, err := os.Stat(probe.Path(dir)); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	lines, err := Lines(root, dir, judgeModel)
	if err != nil {
		return false, err
	}
	for _, line := range lines {
		if line.Verdict.Scenario == sc.Name && line.Verdict.Binding == binding {
			return line.Current(), nil
		}
	}
	return false, nil
}
