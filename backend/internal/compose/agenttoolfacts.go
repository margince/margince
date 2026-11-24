// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"
)

// AgentToolFact is what a model reads of one tool and what bounds its use. The
// spec itself holds functions and cannot be encoded, so the fields a window
// turns on are named here.
type AgentToolFact struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Scope        string          `json:"scope"`
	Tier         string          `json:"tier"`
	ReadOnly     bool            `json:"read_only"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

// AgentToolFacts is the goal of a scheduled agent and the published spec of each
// tool it may use. A check that replays a run reads the tools only through
// these, so it hashes them where it would otherwise walk the registry's builder
// and every store behind it.
func AgentToolFacts(agent string) (goal string, tools []AgentToolFact, err error) {
	spec, ok := ScheduledAgentSpecByName(agent)
	if !ok {
		return "", nil, fmt.Errorf("compose: the runner catalog schedules no agent %q", agent)
	}
	registry := NewRegistry(nil, SendPath{})
	for _, name := range spec.Tools {
		tool, known := registry.Spec(name)
		if !known {
			return "", nil, fmt.Errorf("compose: agent %s may use %s, which the tool registry does not serve", agent, name)
		}
		tools = append(tools, AgentToolFact{
			Name: name, Description: tool.Description, Scope: string(tool.RequiredScope), Tier: fmt.Sprint(tool.Tier),
			ReadOnly: tool.ReadOnly(), InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema,
		})
	}
	return spec.Goal, tools, nil
}
