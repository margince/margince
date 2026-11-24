// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentloope2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Criterion is the judge's answer to one yes/no question about a run.
type Criterion struct {
	Text   string `json:"criterion"`
	Met    bool   `json:"met"`
	Reason string `json:"reason"`
}

// Run is one run's outcome: the checks its end state failed, and the judge's
// answer to each criterion.
type Run struct {
	Failures []Failure   `json:"failures"`
	Criteria []Criterion `json:"criteria"`
}

// ChecksPassed says the end state failed no check.
func (r Run) ChecksPassed() bool { return len(r.Failures) == 0 }

// JudgePassed says the judge answered yes to every criterion. A run the judge
// was never asked about does not pass it.
func (r Run) JudgePassed() bool {
	return len(r.Criteria) > 0 && !slices.ContainsFunc(r.Criteria, func(c Criterion) bool { return !c.Met })
}

// smokeRuns is the number of runs a verdict needs before it is more than a
// smoke result. Three runs only catch a gross failure.
const smokeRuns = 12

// Outcome is the verdict on a case from its runs.
type Outcome struct {
	Runs int `json:"runs"`
	// Passed counts the runs that passed their checks and the judge.
	Passed int  `json:"passed"`
	Pass   bool `json:"pass"`
	// Smoke says the runs are too few to certify anything.
	Smoke bool `json:"smoke"`
}

// Decide applies the pass bar: every run passes its checks, and at least two
// thirds of them, rounded up, also pass the judge. A run that failed a check
// does not count as a judge pass whatever the judge said.
func Decide(runs []Run) Outcome {
	out := Outcome{Runs: len(runs), Smoke: len(runs) < smokeRuns}
	allChecked := len(runs) > 0
	for _, run := range runs {
		switch {
		case !run.ChecksPassed():
			allChecked = false
		case run.JudgePassed():
			out.Passed++
		}
	}
	need := (2*len(runs) + 2) / 3
	out.Pass = allChecked && out.Passed >= need
	return out
}

// Binding is what a verdict was measured on.
type Binding struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Tier     string `json:"tier"`
}

// Key is the binding as one string in which no two bindings meet. Each part is
// escaped, and the parts are joined by an "@" that the escape never leaves in a
// part.
func (b Binding) Key() string {
	return strings.Join([]string{url.QueryEscape(b.Provider), url.QueryEscape(b.Model), url.QueryEscape(b.Tier)}, "@")
}

// Stamp is what one verdict measured. Scenario is the digest of the scenario
// file whole, seed included. Window is the digest of the agent's goal and its
// allowed tool specs. Harness is the digest of the lane's own files. Binding is
// the binding's key and Judge is the judge model's name. A verdict whose part no
// longer matches names the part that moved.
type Stamp struct {
	Scenario string `json:"scenario"`
	Window   string `json:"window"`
	Harness  string `json:"harness"`
	Binding  string `json:"binding"`
	Judge    string `json:"judge"`
}

// NewStamp builds a stamp, taking the binding and the judge from the same
// values the verdict records.
func NewStamp(scenario, window, harness string, binding Binding, judge string) Stamp {
	return Stamp{Scenario: scenario, Window: window, Harness: harness, Binding: binding.Key(), Judge: judge}
}

// Moved names the parts of a recorded stamp that no longer match the current
// one, in a fixed order. Empty means the verdict still describes the product.
func (s Stamp) Moved(current Stamp) []string {
	var moved []string
	for _, part := range []struct{ name, recorded, want string }{
		{"scenario", s.Scenario, current.Scenario},
		{"agent window", s.Window, current.Window},
		{"harness", s.Harness, current.Harness},
		{"binding", s.Binding, current.Binding},
		{"judge", s.Judge, current.Judge},
	} {
		if part.recorded != part.want {
			moved = append(moved, part.name)
		}
	}
	return moved
}

// Verdict is one committed verdict file. It has no `task` key, so the
// certification records never read it as one of theirs.
type Verdict struct {
	Scenario   string  `json:"scenario"`
	Agent      Agent   `json:"agent"`
	Binding    Binding `json:"binding"`
	Judge      string  `json:"judge"`
	SelfJudged bool    `json:"self_judged"`
	Runs       []Run   `json:"run_results"`
	Outcome    Outcome `json:"outcome"`
	Stamp      Stamp   `json:"stamp"`
	RanAt      string  `json:"ran_at"`
}

// Path is where a verdict lives under root: one folder per binding.
func (v Verdict) Path(root string) string {
	return filepath.Join(root, v.Binding.Key(), v.Scenario+".json")
}

// Write files a verdict under root with a stable layout. It refuses a scenario
// name that is not a plain file name, and a binding with a part missing. Either
// would file the verdict somewhere no reader looks.
func (v Verdict) Write(root string) error {
	if !safeName.MatchString(v.Scenario) {
		return fmt.Errorf("agentloope2e: the scenario name %q is not a plain file name", v.Scenario)
	}
	if v.Binding.Provider == "" || v.Binding.Model == "" || v.Binding.Tier == "" {
		return errors.New("agentloope2e: a verdict names its provider, model and tier")
	}
	path := v.Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

// ReadVerdicts reads every verdict under root, in path order. A missing root
// holds none yet. A file that is not where its own content says it belongs is
// refused, so a moved or renamed verdict cannot be read as another's.
func ReadVerdicts(root string) ([]Verdict, error) {
	folders, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Verdict
	for _, folder := range folders {
		if !folder.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, folder.Name()))
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
				continue
			}
			v, err := readVerdict(root, filepath.Join(root, folder.Name(), file.Name()))
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func readVerdict(root, path string) (Verdict, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- a verdict file under the lane's own folder
	if err != nil {
		return Verdict{}, err
	}
	var v Verdict
	if err := json.Unmarshal(raw, &v); err != nil {
		return Verdict{}, fmt.Errorf("agentloope2e: %s: %w", path, err)
	}
	if v.Path(root) != path {
		return Verdict{}, fmt.Errorf("agentloope2e: %s holds the verdict of %s under %s, so it is in the wrong place", path, v.Scenario, v.Path(root))
	}
	return v, nil
}
