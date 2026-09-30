// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The step a scenario expects the turn to take, and the spellings that let it
// say how much of that step it means.
//
// `answer` is a step name, and for most scenarios that is the whole claim: the
// turn had to read before it wrote, and which arguments it read with is the
// model's business. A scenario that DOES care — the grounding identifies a
// record, so a call omitting it is a call that guessed — needs somewhere to put
// that, and pinning it inside the step name would make every other scenario
// carry an assertion it does not mean.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// agentLoopRegistry is the registered surface, resolved once: the set a
// passport admitting every scope is offered, which the runner then narrows to
// the agent's allowlist. Built from the same NewRegistry every other sweep over
// this surface builds, with no pool — registration is composition-time and
// needs no database, and a certification turn executes nothing.
var agentLoopRegistry = sync.OnceValue(func() []mcp.ToolSpec {
	return NewRegistry(nil, SendPath{}).Specs()
})

// agentLoopStep is what a scenario says the turn must do: the step it takes,
// and — only when the scenario means it — the arguments that step has to carry.
type agentLoopStep struct {
	name string
	args map[string]json.RawMessage
}

// UnmarshalJSON reads the two spellings of an expected step. A bare name asserts
// the step and nothing about how it was called; the object form adds arguments,
// and exists so the scenarios that DO pin one are the only ones that carry it.
func (s *agentLoopStep) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &s.name); err == nil {
		return nil
	}
	var object struct {
		Step string                     `json:"step"`
		Args map[string]json.RawMessage `json:"args"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&object); err != nil {
		return fmt.Errorf(
			"%s: the expected answer is not a step name, and not a {step, args} object either: %w",
			agentLoopSite, err)
	}
	s.name, s.args = object.Step, object.Args
	return nil
}

// refuseUnaskableArguments names an argument assertion the turn could never
// satisfy, for a reason that is the scenario's rather than the model's.
//
// Two of them. The step that ends a run carries no arguments at all, so pinning
// one there asserts something the protocol has no room for. And a tool is called
// by the schema the window prints, so an argument that schema does not declare
// is one no correct call would carry — which is exactly how a scenario comes to
// pin `owner` against a tool that takes `owner_id` and reads as a model failure
// ever after.
func refuseUnaskableArguments(want agentLoopStep, specs []mcp.ToolSpec) error {
	if len(want.args) == 0 {
		return nil
	}
	if want.name == agentLoopFinalStep {
		return fmt.Errorf(
			"%s: the scenario pins arguments on %q, and the step that ends a run carries none",
			agentLoopSite, agentLoopFinalStep)
	}
	for _, spec := range specs {
		if spec.Name != want.name {
			continue
		}
		declared := agentLoopSchemaProperties(spec.InputSchema)
		for _, arg := range sortedArgNames(want.args) {
			if !declared[arg] {
				return fmt.Errorf(
					"%s: the scenario pins the argument %q on %s, which its input schema does not declare",
					agentLoopSite, arg, want.name)
			}
		}
	}
	return nil
}

// agentLoopSchemaProperties names what a tool's advertised schema lets a caller
// pass. A schema with no properties object names nothing, which refuses every
// pinned argument — the right answer for a tool the window prints as taking none.
func agentLoopSchemaProperties(schema json.RawMessage) map[string]bool {
	var object struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &object); err != nil {
		return nil
	}
	declared := make(map[string]bool, len(object.Properties))
	for name := range object.Properties {
		declared[name] = true
	}
	return declared
}

// agentLoopArgDisagreements names every pinned argument the call did not carry
// as pinned. All of them, not the first: a call that got one argument right and
// two wrong is not the near miss one line would read as.
//
// It is a SUBSET claim, like every other expectation in this tree. An argument
// the scenario never named is not a disagreement — a real call carries the
// optional arguments its author never thought to pin, and demanding
// exhaustiveness would fail a correct call for being more complete than the
// scenario imagined.
func agentLoopArgDisagreements(want map[string]json.RawMessage, called json.RawMessage) []string {
	// A scenario that pins nothing asserts nothing about the call, so there is
	// nothing here to disagree with — including when the step that was taken
	// carries no arguments at all.
	if len(want) == 0 {
		return nil
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(called, &got); err != nil {
		return []string{"the call carried no argument object to compare"}
	}
	out := make([]string, 0, len(want))
	for _, arg := range sortedArgNames(want) {
		actual, present := got[arg]
		if !present {
			out = append(out, fmt.Sprintf("%s was not passed", arg))
			continue
		}
		if !agentLoopSameJSON(want[arg], actual) {
			out = append(out, fmt.Sprintf("%s was passed as %s, not %s", arg, actual, want[arg]))
		}
	}
	return out
}

// agentLoopSameJSON compares two encoded values by what they mean rather than
// how they were written, so a scenario neither fails on re-ordered object keys
// nor passes on a different value that happens to share a prefix.
//
// Numbers are read as json.Number and compared as exact rationals, which is the
// only reading that answers both halves of "what they mean". Decoding into any
// yields float64, and two integers a scenario could legitimately pin — a limit
// past 2^53 — collapse onto the same float and would be graded equal while
// differing. Comparing the literals instead trades that for the opposite error,
// where a call passing 5.0 against a pinned 5 reads as a disagreement about a
// value both sides agree on.
func agentLoopSameJSON(want, got json.RawMessage) bool {
	wantValue, err := agentLoopDecode(want)
	if err != nil {
		return false
	}
	gotValue, err := agentLoopDecode(got)
	if err != nil {
		return false
	}
	return agentLoopSameValue(wantValue, gotValue)
}

// agentLoopDecode reads one encoded value with numbers left as written, so the
// comparison below decides what they mean rather than inheriting float64's
// answer.
//
//craft:ignore naked-any a decoded JSON document is any by construction — the argument being compared is whatever the tool's schema admits, and encoding/json returns exactly this type.
func agentLoopDecode(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// agentLoopSameValue walks two decoded values together. It is a walk rather
// than reflect.DeepEqual because only the walk reaches the numbers: a pinned
// argument may be an object or a list, and a number nested in one is the same
// claim as a number that is one.
//
//craft:ignore naked-any it walks a decoded JSON document, whose nodes are the any that encoding/json produces — a narrower parameter would describe a shape the corpus is free not to use.
func agentLoopSameValue(want, got any) bool {
	switch wantValue := want.(type) {
	case json.Number:
		gotNumber, ok := got.(json.Number)
		return ok && agentLoopSameNumber(wantValue, gotNumber)
	case map[string]any:
		gotMap, ok := got.(map[string]any)
		if !ok || len(wantValue) != len(gotMap) {
			return false
		}
		for key, value := range wantValue {
			other, present := gotMap[key]
			if !present || !agentLoopSameValue(value, other) {
				return false
			}
		}
		return true
	case []any:
		gotList, ok := got.([]any)
		if !ok || len(wantValue) != len(gotList) {
			return false
		}
		for i, value := range wantValue {
			if !agentLoopSameValue(value, gotList[i]) {
				return false
			}
		}
		return true
	default:
		// Strings, booleans and null carry no representation to normalize, so
		// they mean what they are.
		return want == got
	}
}

// agentLoopSameNumber compares two JSON numbers exactly. A rational parses every
// literal JSON admits without rounding any of them, so 5 equals 5.0 and two
// integers past float64's reach stay apart.
func agentLoopSameNumber(want, got json.Number) bool {
	wantRat, wantOK := new(big.Rat).SetString(want.String())
	gotRat, gotOK := new(big.Rat).SetString(got.String())
	return wantOK && gotOK && wantRat.Cmp(gotRat) == 0
}

// sortedKeys reads a map in one order, so a refusal names the same argument
// first however the scenario was written.
func sortedArgNames(args map[string]json.RawMessage) []string {
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
