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
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

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
		served := copySchemas(spec)
		served.InsteadTools = r.insteadTools(spec)
		out = append(out, served)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// insteadTools lists the other registered tools a spec's Instead names, read
// at listing time rather than at registration, because the neighbour a
// sentence names may register after the tool naming it. The caller holds r.mu.
func (r *Registry) insteadTools(spec mcp.ToolSpec) []string {
	var named []string
	for _, word := range strings.FieldsFunc(spec.Instead, outsideToolName) {
		if _, registered := r.specs[word]; registered && word != spec.Name && !slices.Contains(named, word) {
			named = append(named, word)
		}
	}
	return named
}

// outsideToolName reports a rune no tool name carries: names are lower-case
// snake_case, so anything else ends one.
func outsideToolName(c rune) bool {
	return c != '_' && !unicode.IsLower(c) && !unicode.IsDigit(c)
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
// advertised and enforces nothing. An Unlisted tool is never offered.
func (r *Registry) Offered(ctx context.Context) []mcp.ToolSpec {
	all := r.Specs()
	out := make([]mcp.ToolSpec, 0, len(all))
	for _, spec := range all {
		if !spec.Unlisted && invocableByCaller(ctx, spec) {
			out = append(out, r.Listed(spec))
		}
	}
	return out
}

// Listed is a spec as the external listing words it. A verb only a router
// reaches is named by that router and action, the call a client can make. A
// Surface-B run is offered the verbs themselves and reads their own names.
func (r *Registry) Listed(spec mcp.ToolSpec) mcp.ToolSpec {
	calls := r.routedCalls()
	for word := range calls {
		// A word the tool takes as one of its own values names that value,
		// not the verb spelled alike.
		if strings.Contains(string(spec.InputSchema), strconv.Quote(word)) {
			delete(calls, word)
		}
	}
	reword := func(text string) string {
		for word, call := range calls {
			text = routedWord(word).ReplaceAllString(text, call)
		}
		return text
	}
	spec.Description = reword(spec.Description)
	spec.InputSchema = rewordDescriptions(spec.InputSchema, reword)
	return spec
}

// rewordDescriptions rewrites the descriptions in a schema, at any depth, and
// leaves every other member as it is.
func rewordDescriptions(schema json.RawMessage, reword func(string) string) json.RawMessage {
	var doc any
	if len(schema) == 0 || json.Unmarshal(schema, &doc) != nil {
		return schema
	}
	changed := false
	var walk func(any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			if d, ok := n["description"].(string); ok {
				if r := reword(d); r != d {
					n["description"], changed = r, true
				}
			}
			for _, v := range n {
				walk(v)
			}
		case []any:
			for _, v := range n {
				walk(v)
			}
		}
	}
	walk(doc)
	if !changed {
		return schema
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return schema
	}
	return out
}

func routedWord(verb string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(verb) + `\b`)
}

// routedCalls maps each verb a field router fronts to the call that reaches
// it, as `router with field=action`.
func (r *Registry) routedCalls() map[string]string {
	out := map[string]string{}
	for _, spec := range r.Specs() {
		if spec.Unlisted || spec.ActionField == "" {
			continue
		}
		for _, a := range spec.Actions {
			out[a.Verb] = spec.Name + " with " + spec.ActionField + "=" + a.Action
		}
	}
	return out
}

// Runnable is what a Surface-B run may choose among: every tool this caller may
// invoke, offered on the external listing or not. A scheduled agent's allowlist
// narrows it to the verbs that agent names, each with its own full copy.
func (r *Registry) Runnable(ctx context.Context) []mcp.ToolSpec {
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

// RouteTargets names the tools a router runs as, and reports false for a tool
// that is not a router. A gate holding routers to the contract reads it: a
// router declares nothing of its own, so its targets' declarations are its own.
func (r *Registry) RouteTargets(name string) ([]string, bool) {
	r.mu.RLock()
	rt, isRouter := r.tools[name].(router)
	r.mu.RUnlock()
	if !isRouter {
		return nil, false
	}
	return slices.Sorted(maps.Values(rt.routeTargets())), true
}
