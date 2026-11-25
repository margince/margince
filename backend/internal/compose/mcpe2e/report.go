// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// VerdictTrees are the committed verdict folders, one per door, relative to
// the repository root.
var VerdictTrees = map[Door]string{
	DoorMCP:  "backend/internal/compose/aicert/records/mcp_e2e",
	DoorREST: "backend/internal/compose/aicert/records/mcp_e2e_rest",
}

// doors lists the doors in a fixed order, for output a diff can read.
var doors = []Door{DoorMCP, DoorREST}

// RecordedStamp is the stamp a verdict carries: what it measured, and when.
// Commit and RanAt are the model run's; GradedCommit and GradedAt are the
// checker's, which differ when saved transcripts were graded again later.
type RecordedStamp struct {
	Stamp
	Commit       string `json:"commit"`
	RanAt        string `json:"ran_at"`
	GradedCommit string `json:"graded_commit,omitempty"`
	GradedAt     string `json:"graded_at,omitempty"`
}

// Regraded is whether a later checker graded the run's saved transcripts.
func (r *RecordedStamp) Regraded() bool {
	return r != nil && r.GradedCommit != "" && r.GradedCommit != r.Commit
}

// Verdict is one committed verdict file: how it went, how it was measured,
// and what it measured. Folder is the model folder it is filed under.
type Verdict struct {
	Door       Door           `json:"-"`
	Tree       string         `json:"-"`
	Folder     string         `json:"-"`
	Scenario   string         `json:"scenario"`
	Passed     int            `json:"passed"`
	Runs       int            `json:"runs"`
	PassAt     int            `json:"pass_at"`
	Driver     string         `json:"driver"`
	Search     string         `json:"search"`
	SelfJudged bool           `json:"self_judged"`
	Stamp      *RecordedStamp `json:"stamp"`
	// Efficiency is nil on a verdict recorded before the lane measured it.
	// Slow is its median calls past the case's `expected_calls`.
	Efficiency *Efficiency `json:"efficiency"`
	Slow       bool        `json:"slow"`
	// ToolsByRun is each run's tools that worked, by bare name, and whether
	// that run passed. ToolsCalled is the older union over every run.
	ToolsByRun  []RunTools `json:"tools_by_run"`
	ToolsCalled []string   `json:"tools_called"`
}

// RunTools is one run's tools that worked, and whether the run passed.
type RunTools struct {
	Passed bool     `json:"passed"`
	Tools  []string `json:"tools"`
}

// PassingTools names the tools a passing run used, sorted. An older verdict
// holds only the union over every run, which is a passing run's only when
// every run passed.
func (v Verdict) PassingTools() []string {
	if v.ToolsByRun == nil {
		if v.Runs > 0 && v.Passed == v.Runs {
			return v.ToolsCalled
		}
		return nil
	}
	var tools []string
	for _, run := range v.ToolsByRun {
		if run.Passed {
			tools = append(tools, run.Tools...)
		}
	}
	slices.Sort(tools)
	return slices.Compact(tools)
}

// Unstamped is the reason a verdict made before the stamp existed gives.
const Unstamped = "no stamp"

// Recheck names why a verdict no longer describes the product. It answers the
// parts that moved, "no stamp" or "no scenario"; empty means it is current.
func Recheck(recorded *RecordedStamp, current Stamp, scenarioExists bool, driver string) []string {
	switch {
	case !scenarioExists:
		return []string{"no scenario"}
	case recorded == nil:
		return []string{Unstamped}
	default:
		return recorded.Moved(current, driver)
	}
}

// ReadVerdicts reads every committed verdict under the trees, in path order.
func ReadVerdicts(root string) ([]Verdict, error) {
	var out []Verdict
	for _, door := range doors {
		verdicts, err := ReadTree(filepath.Join(root, VerdictTrees[door]))
		if err != nil {
			return nil, err
		}
		for i := range verdicts {
			verdicts[i].Door = door
		}
		out = append(out, verdicts...)
	}
	return out, nil
}

// ReadTree reads one verdict tree, one folder per model; a missing tree holds
// no verdict yet. A nested folder means a model id carried a '/'. A folder that
// mixes lexical verdicts with others would publish two experiments as one.
// Both are refused.
func ReadTree(dir string) ([]Verdict, error) {
	models, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Verdict
	for _, model := range models {
		if !model.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, model.Name()))
		if err != nil {
			return nil, err
		}
		searches := map[string]bool{}
		for _, file := range files {
			if file.IsDir() {
				return nil, fmt.Errorf("%s/%s: a verdict folder may not nest; the model id carried a '/', "+
					"so file it under E2E_LLM_FOLDER", model.Name(), file.Name())
			}
			if !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			v := Verdict{Tree: filepath.Base(dir), Folder: model.Name()}
			if err := readJSON(filepath.Join(dir, model.Name(), file.Name()), &v); err != nil {
				return nil, err
			}
			searches[v.Search] = true
			out = append(out, v)
		}
		if searches["lexical"] && len(searches) > 1 {
			return nil, fmt.Errorf("%s mixes lexical verdicts with others; a lexical run files under "+
				"E2E_LLM_VERDICTS, never in the records tree", model.Name())
		}
	}
	return out, nil
}

// Line is one verdict's state in a report.
type Line struct {
	Verdict
	Why []string
}

// Report pairs each committed verdict with what makes it stale, if anything.
func Report(root string) ([]Line, error) {
	current, err := stampsByDoor(root)
	if err != nil {
		return nil, err
	}
	verdicts, err := ReadVerdicts(root)
	if err != nil {
		return nil, err
	}
	out := make([]Line, 0, len(verdicts))
	for _, v := range verdicts {
		stamp, exists := current[v.Door][v.Scenario]
		out = append(out, Line{Verdict: v, Why: Recheck(v.Stamp, stamp, exists, v.Driver)})
	}
	return out, nil
}

func stampsByDoor(root string) (map[Door]map[string]Stamp, error) {
	out := map[Door]map[string]Stamp{}
	for _, door := range doors {
		stamps, err := CurrentStamps(root, door)
		if err != nil {
			return nil, err
		}
		out[door] = stamps
	}
	return out, nil
}

// NewlyStale is what a change makes stale: the verdicts under head that were
// current against the base tree's files and are not against head's.
func NewlyStale(baseRoot, headRoot string) ([]Line, error) {
	base, err := stampsByDoor(baseRoot)
	if err != nil {
		return nil, fmt.Errorf("the base tree: %w", err)
	}
	lines, err := Report(headRoot)
	if err != nil {
		return nil, err
	}
	var out []Line
	for _, line := range lines {
		was, existed := base[line.Door][line.Scenario]
		if len(line.Why) > 0 && len(Recheck(line.Stamp, was, existed, line.Driver)) == 0 {
			out = append(out, line)
		}
	}
	return out, nil
}

// Summarise renders the report for reading: per folder, how many verdicts are
// current, then each stale one and why.
func Summarise(lines []Line) string {
	type tally struct{ current, stale int }
	counts := map[string]*tally{}
	var stale, regraded []string
	for _, line := range lines {
		key := line.Tree + "/" + line.Folder
		if counts[key] == nil {
			counts[key] = &tally{}
		}
		if line.Stamp.Regraded() {
			regraded = append(regraded, fmt.Sprintf("  re-graded  %s %s: run at %s, graded at %s",
				key, line.Scenario, line.Stamp.Commit, line.Stamp.GradedCommit))
		}
		if len(line.Why) == 0 {
			counts[key].current++
			continue
		}
		counts[key].stale++
		stale = append(stale, fmt.Sprintf("  stale  %s %s: %s", key, line.Scenario, strings.Join(line.Why, ", ")))
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s: %d current, %d stale\n", key, counts[key].current, counts[key].stale)
	}
	for _, detail := range append(stale, regraded...) {
		b.WriteString(detail + "\n")
	}
	return b.String()
}

// WriteStamps writes the current stamps as JSON for the lane to file beside
// each verdict.
func WriteStamps(root string, door Door, path string) error {
	stamps, err := CurrentStamps(root, door)
	if err != nil {
		return err
	}
	return writeJSON(path, stamps)
}

func writeJSON(path string, value map[string]Stamp) error {
	body, err := marshalIndent(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}
