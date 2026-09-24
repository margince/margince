// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The read-only half of the registry: what a caller may ask about a
// registered tool without invoking it — its record-type shape, its spec,
// and the catalog a listing draws from. Split from registry.go, which owns
// the admission and dispatch path; these answer questions ABOUT that
// surface rather than running it.

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"sort"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// NamesRecordType reports whether this verb can say which record type a given
// call acts on, which is the only way the contract's per-record-type floor is
// ever consulted for it (tierfloor.go). Exported for the same gate as Stageable:
// a floor declared for a verb that cannot answer binds nothing, and reads green.
func (r *Registry) NamesRecordType(name string) bool {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	_, typed := t.(recordTypedTool)
	return typed
}

// Performs reports whether this verb can carry out its effect for this record
// type. The composition root derives the tier floor from it: tightening a pair
// the verb cannot serve would turn an instant refusal into an approval a human
// spends on a call that dies at the provider.
func (r *Registry) Performs(name, recordType string) bool {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	typed, isTyped := t.(recordTypedTool)
	return isTyped && typed.ServesRecordType(recordType)
}

// RecordTypeOfCall answers the record type this verb would resolve for these
// arguments, or "" when the verb names none.
//
// Exported for the composition's own gate on genericRecordVerbs: whether a verb
// READS its record type or answers a constant is what decides if canonical-route
// arbitration applies to it, and that is a fact about the tool rather than a list
// a reader has to keep true by hand.
func (r *Registry) RecordTypeOfCall(name string, args json.RawMessage) string {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return ""
	}
	typed, isTyped := t.(recordTypedTool)
	if !isTyped {
		return ""
	}
	return typed.RecordTypeOf(args)
}

// Spec returns the registered spec for name — the REST admission path
// (ADR-0055) resolves a mutating operation's tool twin through this.
func (r *Registry) Spec(name string) (mcp.ToolSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.specs[name]
	return copySchemas(spec), ok
}

// copySchemas hands out a spec whose schemas are the CALLER's bytes.
//
// A json.RawMessage is a slice, so returning the registered one shares its
// backing array: a caller that wrote through it would rewrite what tools/list
// advertises and what results are validated against, for every later request,
// from outside the lock. Every reference-typed member — the two schemas, the
// view declaration and the unkeyed-argument declaration — is copied below;
// everything else on a ToolSpec is copied by the assignment.
func copySchemas(spec mcp.ToolSpec) mcp.ToolSpec {
	spec.InputSchema = bytes.Clone(spec.InputSchema)
	spec.OutputSchema = bytes.Clone(spec.OutputSchema)
	// The view declaration, for the SAME reason and one field over: it is a
	// pointer to a struct holding a slice, so a tool that kept a reference to
	// what it registered could rewrite the URI a host is told to fetch — and the
	// audience it is told to offer the tool to — for every later request, from
	// outside the lock and after the boot-time gate that validated it.
	// And the unkeyed-argument declaration, a map: shared, a caller writing
	// through it could clear the one statement keeping a tool from an agent.
	spec.UnkeyedArguments = maps.Clone(spec.UnkeyedArguments)
	if spec.UI != nil {
		ui := *spec.UI
		ui.Visibility = append([]string(nil), ui.Visibility...)
		spec.UI = &ui
	}
	return spec
}

// Specs lists the registered surface, stably ordered for tools/list.
func (r *Registry) Specs() []mcp.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]mcp.ToolSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		out = append(out, copySchemas(spec))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Offered is the surface THIS caller may invoke: the catalog both the external
// tools/list and a Surface-B run's tool listing are drawn from.
//
// One function serves both, rather than two filters that agree today. A run
// offered a verb its passport cannot spend is being asked to choose among names
// it will be refused for, and every one of them rides in a system prompt that
// elision never touches.
//
// It answers the SCOPE axis only. Invoke remains the authority on the seat
// ceiling and object RBAC, which are re-derived per call — this narrows what is
// advertised and enforces nothing.
func (r *Registry) Offered(ctx context.Context) []mcp.ToolSpec {
	all := r.Specs()
	out := make([]mcp.ToolSpec, 0, len(all))
	for _, spec := range all {
		if invocableByCaller(ctx, spec) {
			out = append(out, spec)
		}
	}
	return out
}

// dynamicTool is implemented by TierDynamic tools that need more than the
// raw args to resolve their tier — advance_deal reads the target stage's
// semantic from pipeline configuration, which costs a database read the
// gate should pay only for dynamic calls.
type dynamicTool interface {
	ResolverInput(ctx context.Context, in json.RawMessage) (mcp.TierResolverInput, error)
}

// UnknownToolError answers a tools/call for a name outside the surface.
type UnknownToolError struct{ Name string }

// maxToolNameEcho bounds the caller-supplied name this error quotes back.
// The name is chosen freely by the model and lands in a transcript that the
// same run's later prompts read, so an unbounded echo is an unbounded write
// into those prompts. Generous next to the longest real tool name, short
// enough that the field cannot carry prose.
const maxToolNameEcho = 64

// Error renders the echo HERE rather than at each surface, so no consumer —
// the tool result, the server log, a future transport — can quote the name
// back raw by forgetting to. Bounded AND escaped: the name is chosen by the
// model, and a newline in it would otherwise open what reads as a new line of
// the transcript that the same run's later prompts go on to read.
func (e *UnknownToolError) Error() string {
	return "unknown tool " + echoSafe(e.Name, maxToolNameEcho)
}
