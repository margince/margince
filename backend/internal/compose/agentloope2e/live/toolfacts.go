// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package live

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/compose"
)

// ToolFact is what a model reads of one tool and what bounds its use. The
// spec itself holds functions and cannot be encoded, so the fields a window
// turns on are named here.
type ToolFact struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Scope        string          `json:"scope"`
	Tier         string          `json:"tier"`
	ReadOnly     bool            `json:"read_only"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

// ToolFacts is the goal of a scheduled agent and the published spec of each
// tool it may use. A check that replays a run reads the tools only through
// these. So it hashes them, and never walks the registry's builder and every
// store behind it.
func ToolFacts(agent string) (goal string, tools []ToolFact, err error) {
	spec, ok := compose.ScheduledAgentSpecByName(agent)
	if !ok {
		return "", nil, fmt.Errorf("live: the runner catalog schedules no agent %q", agent)
	}
	registry := compose.NewRegistry(nil, compose.SendPath{})
	for _, name := range spec.Tools {
		tool, known := registry.Spec(name)
		if !known {
			return "", nil, fmt.Errorf("live: agent %s may use %s, which the tool registry does not serve", agent, name)
		}
		tools = append(tools, ToolFact{
			Name: name, Description: tool.Description, Scope: string(tool.RequiredScope), Tier: fmt.Sprint(tool.Tier),
			ReadOnly: tool.ReadOnly(), InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema,
		})
	}
	return spec.Goal, tools, nil
}
