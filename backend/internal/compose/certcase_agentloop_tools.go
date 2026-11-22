// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tool surface a certification run of a scheduled agent calls. Every read
// is answered from the scenario's fixture. The agent's one write is checked as
// its store checks it, then acknowledged and never performed.
//
// The lane has no database, and what is certified is what the model does with
// what the tools said. The worklist is decoded into the production result type
// and re-encoded, so a fixture cannot serve a shape the tool never does.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// agentLoopToolAnswer is what one tool call is answered with. Args, when set,
// is a subset the call must carry to get this answer. So two reads of one tool
// on two records are two entries.
type agentLoopToolAnswer struct {
	Tool   string                     `json:"tool"`
	Args   map[string]json.RawMessage `json:"args,omitempty"`
	Result json.RawMessage            `json:"result,omitempty"`
}

// agentLoopToolSurface is the tool surface a certification run is offered. It
// advertises the registered surface, as a passport admitting every scope would.
// The runner narrows it to the agent's allowlist as it narrows a production run's. It holds no state, so a replay of the same replies meets
// the same answers.
type agentLoopToolSurface struct {
	work    agentLoopWork
	list    agentLoopWorklist
	answers []agentLoopToolAnswer
}

func (agentLoopToolSurface) Specs() []mcp.ToolSpec { return agentLoopRegistry() }

func (agentLoopToolSurface) Runnable(context.Context) []mcp.ToolSpec { return agentLoopRegistry() }

func (agentLoopToolSurface) Explain(_ context.Context, _ string, err error) string {
	return agents.RunRefusal(err)
}

// Invoke answers one call. A read no fixture entry matches is ErrNotFound, the
// existence-hiding answer a read of a record nobody wrote gets in production.
func (s agentLoopToolSurface) Invoke(_ context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	args, err := agents.WithoutReservedArgs(args)
	if err != nil {
		return nil, err
	}
	if name == s.work.write {
		if _, err := s.work.wrote(args, s.list); err != nil {
			return nil, err
		}
		return s.work.acknowledge(args, s.fixedAnswer(name, args))
	}
	if name == s.work.list {
		return s.list.normalized, nil
	}
	if answer := s.fixedAnswer(name, args); answer != nil {
		return answer, nil
	}
	return nil, apperrors.ErrNotFound
}

// fixedAnswer is the first fixture entry for tool whose pinned arguments the
// call carries, or nil when none does.
func (s agentLoopToolSurface) fixedAnswer(tool string, args json.RawMessage) json.RawMessage {
	for _, answer := range s.answers {
		if answer.Tool == tool && len(agentLoopArgDisagreements(answer.Args, args)) == 0 {
			return answer.Result
		}
	}
	return nil
}

// refuseUnanswerableTools names a fixture answer no call of this run could
// reach. That is an answer for a tool the agent is not offered, or one pinning
// an argument the tool's schema does not declare.
func refuseUnanswerableTools(answers []agentLoopToolAnswer, specs []mcp.ToolSpec) error {
	offered := map[string]mcp.ToolSpec{}
	for _, spec := range specs {
		offered[spec.Name] = spec
	}
	for _, answer := range answers {
		spec, ok := offered[answer.Tool]
		if !ok {
			return fmt.Errorf("%s: the fixture answers %q, which this agent is not offered", agentLoopSite, answer.Tool)
		}
		declared := agentLoopSchemaProperties(spec.InputSchema)
		for _, arg := range sortedArgNames(answer.Args) {
			if !declared[arg] {
				return fmt.Errorf("%s: the fixture's answer for %s pins %q, which its input schema does not declare",
					agentLoopSite, answer.Tool, arg)
			}
		}
	}
	return nil
}

// listAnswer is the fixture's one answer to the agent's list read. The list is
// the run's whole worklist, and two answers would leave the grade to whichever
// the model's arguments matched.
func listAnswer(answers []agentLoopToolAnswer, list string) (json.RawMessage, error) {
	var found []json.RawMessage
	for _, answer := range answers {
		if answer.Tool == list {
			found = append(found, answer.Result)
		}
	}
	if len(found) != 1 {
		return nil, fmt.Errorf("%s: the fixture answers %s %d times; the run's worklist is exactly one answer to it",
			agentLoopSite, list, len(found))
	}
	return found[0], nil
}

// decodeStrict reads raw into the production type v, refusing a field that
// type does not hold: a fixture answer the tool could not have produced.
//
//craft:ignore naked-any it decodes into whichever production result type the caller passes, as encoding/json does.
func decodeStrict(raw json.RawMessage, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// writeTool is the agent's one tool that changes a record, derived from the
// registry's own scope on each allowlisted tool. An agent whose allowlist holds
// none or two has no single write a run of it could be graded on.
func writeTool(allowlist []string, specs []mcp.ToolSpec) (string, error) {
	var writes []string
	for _, spec := range specs {
		if slices.Contains(allowlist, spec.Name) && !spec.ReadOnly() {
			writes = append(writes, spec.Name)
		}
	}
	if len(writes) != 1 {
		return "", fmt.Errorf("%s: this agent attaches %d writing tools (%s), and a run is graded on its one write",
			agentLoopSite, len(writes), strings.Join(writes, ", "))
	}
	return writes[0], nil
}

// WalkReplies is a reply calling each tool the fixture answers, in its order,
// and then one that ends the run.
func (c *agentLoopCase) WalkReplies() []string {
	replies := make([]string, 0, len(c.tools.answers)+1)
	for _, answer := range c.tools.answers {
		args := answer.Args
		if args == nil {
			args = map[string]json.RawMessage{}
		}
		step, err := json.Marshal(struct {
			Tool string                     `json:"tool"`
			Args map[string]json.RawMessage `json:"args"`
		}{answer.Tool, args})
		if err == nil {
			replies = append(replies, string(step))
		}
	}
	return append(replies, `{"final":{"summary":"walked every answered tool"}}`)
}
