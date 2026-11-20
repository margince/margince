// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mcpe2e

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// VerdictTrees are the committed verdict folders, one per door, relative to
// the repository root.
var VerdictTrees = []string{
	"backend/internal/compose/aicert/records/mcp_e2e",
	"backend/internal/compose/aicert/records/mcp_e2e_rest",
}

// RecordedStamp is the stamp a verdict carries: what it measured, and when.
type RecordedStamp struct {
	Stamp
	Commit string `json:"commit"`
	RanAt  string `json:"ran_at"`
}

// Verdict is one committed verdict file, as far as its age is concerned.
type Verdict struct {
	Tree     string
	Folder   string
	Scenario string
	Stamp    *RecordedStamp
}

// Unstamped is the reason a verdict made before the stamp existed gives.
const Unstamped = "no stamp"

// Recheck names why a verdict no longer describes the product: the parts that
// moved, "no stamp" for a verdict that cannot say what it measured, or "no
// scenario" for one whose scenario is gone. Empty means it is current.
func Recheck(recorded *RecordedStamp, current Stamp, scenarioExists bool) []string {
	switch {
	case !scenarioExists:
		return []string{"no scenario"}
	case recorded == nil:
		return []string{Unstamped}
	default:
		return recorded.Moved(current)
	}
}

// ReadVerdicts reads every committed verdict under the trees, in path order.
func ReadVerdicts(root string) ([]Verdict, error) {
	var out []Verdict
	for _, tree := range VerdictTrees {
		files, err := filepath.Glob(filepath.Join(root, tree, "*", "*.json"))
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			var v struct {
				Scenario string         `json:"scenario"`
				Stamp    *RecordedStamp `json:"stamp"`
			}
			if err := readJSON(file, &v); err != nil {
				return nil, err
			}
			out = append(out, Verdict{
				Tree: filepath.Base(tree), Folder: filepath.Base(filepath.Dir(file)),
				Scenario: v.Scenario, Stamp: v.Stamp,
			})
		}
	}
	return out, nil
}

// Line is one verdict's state in a report.
type Line struct {
	Verdict
	Why []string
}

// Report is every committed verdict with what makes it stale, if anything.
func Report(root string) ([]Line, error) {
	current, err := CurrentStamps(root)
	if err != nil {
		return nil, err
	}
	verdicts, err := ReadVerdicts(root)
	if err != nil {
		return nil, err
	}
	out := make([]Line, 0, len(verdicts))
	for _, v := range verdicts {
		stamp, exists := current[v.Scenario]
		out = append(out, Line{Verdict: v, Why: Recheck(v.Stamp, stamp, exists)})
	}
	return out, nil
}

// NewlyStale is what a change makes stale: the verdicts under head that were
// current against the base tree's files and are not against head's.
func NewlyStale(baseRoot, headRoot string) ([]Line, error) {
	base, err := CurrentStamps(baseRoot)
	if err != nil {
		return nil, fmt.Errorf("the base tree: %w", err)
	}
	lines, err := Report(headRoot)
	if err != nil {
		return nil, err
	}
	var out []Line
	for _, line := range lines {
		was, existed := base[line.Scenario]
		if len(line.Why) > 0 && len(Recheck(line.Stamp, was, existed)) == 0 {
			out = append(out, line)
		}
	}
	return out, nil
}

// Summarise prints a report a person reads: per folder, how many verdicts are
// current, then each stale one and why.
func Summarise(lines []Line) string {
	type tally struct{ current, stale int }
	counts := map[string]*tally{}
	var stale []string
	for _, line := range lines {
		key := line.Tree + "/" + line.Folder
		if counts[key] == nil {
			counts[key] = &tally{}
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
	b.WriteString(strings.Join(stale, "\n"))
	if len(stale) > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

// WriteStamps writes the current stamps as JSON for the lane to file beside
// each verdict.
func WriteStamps(root, path string) error {
	stamps, err := CurrentStamps(root)
	if err != nil {
		return err
	}
	return writeJSON(path, stamps)
}

func writeJSON(path string, value any) error {
	body, err := marshalIndent(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}
