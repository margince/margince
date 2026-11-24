// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package live

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/agentloope2e"
	"github.com/margince/margince/backend/internal/compose/mcpe2e"
)

// Where the lane's files sit, relative to the repository root.
const (
	// ScenarioDir holds the scenario files.
	ScenarioDir = "backend/internal/compose/agentloope2e/scenarios"
	// VerdictDir holds the verdict files. It sits beside the certification
	// records and holds no record, so readers of those never see it.
	VerdictDir = "backend/internal/compose/aicert/records/agent_loop_e2e"
	// HarnessDir holds the integration harness the verdict was measured with.
	HarnessDir = "backend/internal/compose/integration/jobfanout"

	lanePackage = "backend/internal/compose/agentloope2e"
)

// judgeFiles hold the judge and the door the lane asks it through.
var judgeFiles = []string{"e2e/llm/judge.py", "e2e/llm/judge_cli.py", "e2e/llm/candidates.py"}

// runnerDir holds the code that frames every step the agent takes: the system
// prompt, the step schema, the fence and the step and token budgets. The window
// digest names the goal and the tools, so a change here moves the harness.
const runnerDir = "backend/internal/modules/agents/runner"

// JudgeName is the judge model a verdict is filed under. A run that used another
// than the pinned judge passes its name, which CommandJudge.Model reads from
// judge.py. An empty name is the pinned judge of e2e/llm/candidates.json.
func JudgeName(root, named string) (string, error) {
	if named != "" {
		return named, nil
	}
	return mcpe2e.JudgeModel(root)
}

// toolFacts is what a model reads of a tool and what bounds its use. The spec
// itself holds functions and cannot be encoded, so the fields a window turns on
// are named here.
type toolFacts struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Scope        string          `json:"scope"`
	Tier         string          `json:"tier"`
	ReadOnly     bool            `json:"read_only"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

// windowOf is the digest of what the agent is asked and what it may use.
func windowOf(goal string, tools []toolFacts) (string, error) {
	body, err := json.Marshal(struct {
		Goal  string      `json:"goal"`
		Tools []toolFacts `json:"tools"`
	}{goal, tools})
	if err != nil {
		return "", err
	}
	return mcpe2e.Digest(body), nil
}

// windowDigest is the window of one scheduled agent: its goal and the published
// spec of each tool it is allowed.
func windowDigest(agent agentloope2e.Agent) (string, error) {
	spec, ok := compose.ScheduledAgentSpecByName(string(agent))
	if !ok {
		return "", fmt.Errorf("live: the runner catalog schedules no agent %q", agent)
	}
	registry := compose.NewRegistry(nil, compose.SendPath{})
	var tools []toolFacts
	for _, name := range spec.Tools {
		tool, known := registry.Spec(name)
		if !known {
			return "", fmt.Errorf("live: agent %s may use %s, which the tool registry does not serve", agent, name)
		}
		tools = append(tools, toolFacts{
			Name: name, Description: tool.Description, Scope: string(tool.RequiredScope), Tier: fmt.Sprint(tool.Tier),
			ReadOnly: tool.ReadOnly(), InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema,
		})
	}
	return windowOf(spec.Goal, tools)
}

// harnessDigest is the digest of the files that build the world and grade a run.
// They are the lane's Go sources, the integration harness and the judge. The lists
// come from the folders, so a new file moves the digest by existing. A scenario
// file is not here, since every scenario's stamp holds its own file whole.
func harnessDigest(root string) (string, error) {
	names := slices.Clone(judgeFiles)
	for _, pattern := range []string{
		filepath.Join(lanePackage, "*.go"),
		filepath.Join(lanePackage, "live", "*.go"),
		filepath.Join(HarnessDir, "agentloop_*_test.go"),
		filepath.Join(HarnessDir, "runner_integration_test.go"),
		filepath.Join(runnerDir, "*.go"),
	} {
		found, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return "", err
		}
		for _, path := range found {
			if strings.HasSuffix(path, "_test.go") && strings.HasPrefix(pattern, filepath.Join(runnerDir)) {
				continue
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return "", err
			}
			names = append(names, filepath.ToSlash(rel))
		}
	}
	if len(names) == len(judgeFiles) {
		return "", fmt.Errorf("live: no lane source or harness file under %s, so nothing would move the harness digest", root)
	}
	slices.Sort(names)
	return mcpe2e.DigestFiles(root, names)
}

// CurrentStamp is the stamp a verdict for this scenario and binding would carry
// if it ran on this tree now, judged by the named model. It needs no database
// and no model.
func CurrentStamp(root, judgeModel string, sc agentloope2e.Scenario, binding agentloope2e.Binding) (agentloope2e.Stamp, error) {
	window, err := windowDigest(sc.Agent)
	if err != nil {
		return agentloope2e.Stamp{}, err
	}
	harness, err := harnessDigest(root)
	if err != nil {
		return agentloope2e.Stamp{}, err
	}
	judge, err := JudgeName(root, judgeModel)
	if err != nil {
		return agentloope2e.Stamp{}, err
	}
	return agentloope2e.NewStamp(sc.Digest, window, harness, binding, judge), nil
}
