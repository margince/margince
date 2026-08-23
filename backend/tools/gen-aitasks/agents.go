// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The agents{} declaration: which tools each scheduled agent of a tool-fed
// task attaches, and the three ways that can be wrong.

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// agentDef is one entry of a task's agents{}: a scheduled agent and the tools
// it attaches. It exists because the tool listing rides in EVERY step of a
// tool-fed window, so what a run may call is also what that run pays for in
// prompt — and only the agent's own goal knows which tools that is.
//
// Tools is required and non-empty. Downstream, an empty allowlist is read as
// "no narrowing", so an agent that lost its list would silently regain the
// whole catalog — the opposite of what declaring it was for.
type agentDef struct {
	Tools []string `yaml:"tools"`
}

// UnmarshalYAML re-reads the mapping through the strict decoder. Without it a
// typo'd `tool:` would leave Tools empty, which is the one failure this
// declaration cannot afford (see strictdecode.go).
func (a *agentDef) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Tools []string `yaml:"tools"`
	}
	if err := decodeMapping(node, &raw); err != nil {
		return fmt.Errorf("agent: %w", err)
	}
	a.Tools = raw.Tools
	return nil
}

// validateAgents holds the agents{} mapping to the three things a runtime
// cannot recover from on its own: an allowlist that is empty, an agent
// declared on a task that assembles no tool-fed window, and a name the Go
// side could not carry.
//
// The tool NAMES are deliberately not checked here. This generator has no
// registry to check them against — that is a composition-time gate, and
// splitting the check would mean two half-answers to one question.
func validateAgents(task string, def taskDef) error {
	if len(def.Agents) == 0 {
		return nil
	}
	if !declaresAnAgentLoopSite(def) {
		return fmt.Errorf(
			"task %q: declares agents but no agent_loop site — only a tool-fed window has a tool listing to attach to, so this allowlist would never be assembled", task)
	}
	for _, agent := range sortedAgentNames(def.Agents) {
		if !siteNameRE.MatchString(agent) {
			return fmt.Errorf("task %q: agent name %q must match %s", task, agent, siteNameRE.String())
		}
		tools := def.Agents[agent].Tools
		if len(tools) == 0 {
			return fmt.Errorf(
				"task %q: agent %q declares no tools — an empty allowlist is read as no narrowing, so this agent would be offered the whole catalog", task, agent)
		}
		seen := make(map[string]bool, len(tools))
		for _, tool := range tools {
			if seen[tool] {
				return fmt.Errorf("task %q: agent %q attaches %q twice", task, agent, tool)
			}
			seen[tool] = true
		}
	}
	return nil
}

// declaresAnAgentLoopSite reports whether this task assembles a tool-fed
// window at all — the only kind of site an allowlist has meaning for.
func declaresAnAgentLoopSite(def taskDef) bool {
	for _, s := range def.Sites {
		if s.Kind == kindAgentLoop {
			return true
		}
	}
	return false
}

// sortedAgentNames is the one place agent iteration order is decided, for the
// reason sortedTaskNames exists: a map has none, and a generated file that
// moves between runs cannot be drift-gated.
func sortedAgentNames(agents map[string]agentDef) []string {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
