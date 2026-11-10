// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// CallAvailability applies one feature policy to listings and execution.
type CallAvailability interface {
	Admits(verb, recordType string) bool
	Context(context.Context) context.Context
}

// WithCallAvailability binds the feature boundary before tool admission.
func WithCallAvailability(available CallAvailability) RegistryOption {
	return func(r *Registry) { r.callAvailability = available }
}

func (r *Registry) callAvailable(name string, args json.RawMessage) bool {
	return r.callAvailability == nil || r.callAvailability.Admits(name, r.RecordTypeOfCall(name, args))
}

func (r *Registry) availableSpecs(specs []mcp.ToolSpec) []mcp.ToolSpec {
	if r.callAvailability == nil {
		return specs
	}
	out := make([]mcp.ToolSpec, 0, len(specs))
	for _, spec := range specs {
		available, ok := r.availableSpec(spec)
		if ok {
			out = append(out, available)
		}
	}
	return out
}

func (r *Registry) availableSpec(spec mcp.ToolSpec) (mcp.ToolSpec, bool) {
	if len(spec.Actions) == 0 {
		return spec, r.callAvailable(spec.Name, nil)
	}
	kept := slices.DeleteFunc(slices.Clone(spec.Actions), func(action mcp.ToolAction) bool { return !r.callAvailable(action.Verb, nil) })
	return r.withAvailableActions(spec, kept)
}

func (r *Registry) withAvailableActions(spec mcp.ToolSpec, kept []mcp.ToolAction) (mcp.ToolSpec, bool) {
	if len(kept) == len(spec.Actions) {
		return spec, true
	}
	if len(kept) == 0 {
		return spec, false
	}
	if spec.ActionField == "" {
		return spec, true
	}
	hidden := hiddenActions(spec.Actions, kept)
	spec.Actions = kept
	schema, err := r.narrowedRouterSchema(spec.Name, spec.InputSchema, kept, hidden)
	if err != nil {
		slog.Warn("router schema could not be narrowed, so it is not offered", "tool", spec.Name, "err", err)
		return spec, false
	}
	spec.InputSchema = schema
	spec.Description = withoutSentencesNaming(spec.Description, hidden)
	spec.Instead = withoutSentencesNaming(spec.Instead, hidden)
	if spec.OutputSchema != nil {
		spec.OutputSchema = targetsOutput(spec.Name, r.actionTargets(kept))
	}
	return spec, true
}
